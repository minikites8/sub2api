package service

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	modelAvailabilitySampleLimit       = 90
	modelAvailabilityTrackedModelLimit = 4096
	modelAvailabilityEventIDLimit      = 65536
)

// ModelAvailabilitySample is one user traffic observation. It is kept in
// memory because the marketplace only needs a lightweight fallback signal
// when an explicit channel monitor has no data for a media model.
type ModelAvailabilitySample struct {
	Success   bool
	CheckedAt time.Time
}

// ModelAvailabilityObservation contains the traffic-based availability for a
// requested model since the current process started.
type ModelAvailabilityObservation struct {
	Model           string
	TotalCalls      int64
	SuccessfulCalls int64
	Availability    float64
	LastCalledAt    time.Time
	Samples         []ModelAvailabilitySample
}

type modelAvailabilityCounter struct {
	model           string
	totalCalls      int64
	successfulCalls int64
	lastCalledAt    time.Time
	samples         []ModelAvailabilitySample
}

// ModelAvailabilityTracker records the result of user calls by requested
// model. Event IDs make retried node sync batches idempotent.
type ModelAvailabilityTracker struct {
	mu         sync.RWMutex
	models     map[string]*modelAvailabilityCounter
	eventIDs   map[string]struct{}
	eventOrder []string
}

func NewModelAvailabilityTracker() *ModelAvailabilityTracker {
	return &ModelAvailabilityTracker{
		models:   make(map[string]*modelAvailabilityCounter),
		eventIDs: make(map[string]struct{}),
	}
}

var defaultModelAvailabilityTracker = NewModelAvailabilityTracker()

// DefaultModelAvailabilityTracker is shared by gateway routes, node sync
// ingestion, and the public model marketplace snapshot.
func DefaultModelAvailabilityTracker() *ModelAvailabilityTracker {
	return defaultModelAvailabilityTracker
}

func normalizeAvailabilityModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// Record adds one completed user call. A completed HTTP 2xx response is a
// successful observation; every other response contributes to the denominator.
func (t *ModelAvailabilityTracker) Record(model string, success bool) {
	t.RecordAt(model, success, time.Now().UTC())
}

// RecordAt adds an observation with its original completion time.
func (t *ModelAvailabilityTracker) RecordAt(model string, success bool, checkedAt time.Time) {
	if t == nil {
		return
	}
	key := normalizeAvailabilityModel(model)
	if key == "" {
		return
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	} else {
		checkedAt = checkedAt.UTC()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recordLocked(key, strings.TrimSpace(model), success, checkedAt)
}

// RecordEvent adds a node-synchronized observation once. It returns true when
// the event changed the aggregate and false when the event was already seen.
func (t *ModelAvailabilityTracker) RecordEvent(eventID, model string, success bool, checkedAt time.Time) bool {
	if t == nil {
		return false
	}
	key := normalizeAvailabilityModel(model)
	eventID = strings.TrimSpace(eventID)
	if key == "" || eventID == "" {
		return false
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	} else {
		checkedAt = checkedAt.UTC()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.eventIDs[eventID]; exists {
		return false
	}
	if len(t.models) >= modelAvailabilityTrackedModelLimit {
		if _, exists := t.models[key]; !exists {
			return false
		}
	}
	t.eventIDs[eventID] = struct{}{}
	t.eventOrder = append(t.eventOrder, eventID)
	if len(t.eventOrder) > modelAvailabilityEventIDLimit {
		oldest := t.eventOrder[0]
		delete(t.eventIDs, oldest)
		t.eventOrder = t.eventOrder[1:]
	}
	t.recordLocked(key, strings.TrimSpace(model), success, checkedAt)
	return true
}

func (t *ModelAvailabilityTracker) recordLocked(key, model string, success bool, checkedAt time.Time) {
	counter := t.models[key]
	if counter == nil {
		if len(t.models) >= modelAvailabilityTrackedModelLimit {
			return
		}
		counter = &modelAvailabilityCounter{model: model}
		t.models[key] = counter
	}
	counter.totalCalls++
	if success {
		counter.successfulCalls++
	}
	if counter.lastCalledAt.IsZero() || checkedAt.After(counter.lastCalledAt) {
		counter.lastCalledAt = checkedAt
	}
	counter.samples = append(counter.samples, ModelAvailabilitySample{Success: success, CheckedAt: checkedAt})
	sort.SliceStable(counter.samples, func(i, j int) bool {
		return counter.samples[i].CheckedAt.Before(counter.samples[j].CheckedAt)
	})
	if len(counter.samples) > modelAvailabilitySampleLimit {
		counter.samples = counter.samples[len(counter.samples)-modelAvailabilitySampleLimit:]
	}
}

func availabilityObservation(counter *modelAvailabilityCounter) ModelAvailabilityObservation {
	if counter == nil {
		return ModelAvailabilityObservation{}
	}
	availability := 0.0
	if counter.totalCalls > 0 {
		availability = float64(counter.successfulCalls) / float64(counter.totalCalls) * 100
	}
	samples := append([]ModelAvailabilitySample(nil), counter.samples...)
	return ModelAvailabilityObservation{
		Model:           counter.model,
		TotalCalls:      counter.totalCalls,
		SuccessfulCalls: counter.successfulCalls,
		Availability:    availability,
		LastCalledAt:    counter.lastCalledAt,
		Samples:         samples,
	}
}

// Snapshot returns the current observation for one model.
func (t *ModelAvailabilityTracker) Snapshot(model string) (ModelAvailabilityObservation, bool) {
	if t == nil {
		return ModelAvailabilityObservation{}, false
	}
	key := normalizeAvailabilityModel(model)
	if key == "" {
		return ModelAvailabilityObservation{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	counter, ok := t.models[key]
	if !ok {
		return ModelAvailabilityObservation{}, false
	}
	return availabilityObservation(counter), true
}

// SnapshotAll returns a stable copy for building a public marketplace view.
func (t *ModelAvailabilityTracker) SnapshotAll() []ModelAvailabilityObservation {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]ModelAvailabilityObservation, 0, len(t.models))
	for _, counter := range t.models {
		out = append(out, availabilityObservation(counter))
	}
	return out
}

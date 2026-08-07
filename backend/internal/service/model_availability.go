package service

import (
	"strings"
	"sync"
	"time"
)

const (
	modelAvailabilitySampleLimit       = 90
	modelAvailabilityTrackedModelLimit = 4096
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
// model. The mutex keeps updates and marketplace snapshots consistent.
type ModelAvailabilityTracker struct {
	mu     sync.RWMutex
	models map[string]*modelAvailabilityCounter
}

func NewModelAvailabilityTracker() *ModelAvailabilityTracker {
	return &ModelAvailabilityTracker{models: make(map[string]*modelAvailabilityCounter)}
}

var defaultModelAvailabilityTracker = NewModelAvailabilityTracker()

// DefaultModelAvailabilityTracker is shared by gateway routes and the public
// model marketplace snapshot.
func DefaultModelAvailabilityTracker() *ModelAvailabilityTracker {
	return defaultModelAvailabilityTracker
}

func normalizeAvailabilityModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// Record adds one completed user call. A completed HTTP 2xx response is a
// successful observation; every other response contributes to the denominator.
func (t *ModelAvailabilityTracker) Record(model string, success bool) {
	if t == nil {
		return
	}
	key := normalizeAvailabilityModel(model)
	if key == "" {
		return
	}
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	counter := t.models[key]
	if counter == nil {
		if len(t.models) >= modelAvailabilityTrackedModelLimit {
			return
		}
		counter = &modelAvailabilityCounter{model: strings.TrimSpace(model)}
		t.models[key] = counter
	}
	counter.totalCalls++
	if success {
		counter.successfulCalls++
	}
	counter.lastCalledAt = now
	counter.samples = append(counter.samples, ModelAvailabilitySample{Success: success, CheckedAt: now})
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

package service

import (
	"math"
	"testing"
)

func TestModelAvailabilityTrackerCalculatesSuccessRate(t *testing.T) {
	tracker := NewModelAvailabilityTracker()
	tracker.Record("  Gpt-Image-2  ", true)
	tracker.Record("gpt-image-2", false)
	tracker.Record("GPT-IMAGE-2", true)

	observation, ok := tracker.Snapshot("gpt-image-2")
	if !ok {
		t.Fatal("expected model observation")
	}
	if observation.TotalCalls != 3 || observation.SuccessfulCalls != 2 {
		t.Fatalf("calls = %d/%d, want 2/3", observation.SuccessfulCalls, observation.TotalCalls)
	}
	if math.Abs(observation.Availability-66.6666666667) > 0.0001 {
		t.Fatalf("availability = %f, want 66.6667", observation.Availability)
	}
	if len(observation.Samples) != 3 {
		t.Fatalf("samples = %d, want 3", len(observation.Samples))
	}
}

func TestAppendTrafficAvailabilityMonitorsUsesMediaFallback(t *testing.T) {
	tracker := NewModelAvailabilityTracker()
	tracker.Record("gpt-image-2", true)
	tracker.Record("gpt-image-2", false)

	groups := []PublicTransitGroup{{
		Name: "images",
		Models: []PublicTransitModel{{
			StandardModel: "gpt-image-2",
			RawModel:      "gpt-image-2",
			BillingMode:   string(BillingModeImage),
		}},
	}}
	monitors := appendTrafficAvailabilityMonitorsWithTracker(groups, nil, tracker)
	if len(monitors) != 1 {
		t.Fatalf("monitors = %d, want 1", len(monitors))
	}
	monitor := monitors[0]
	if monitor.PrimaryStatus != MonitorStatusFailed || monitor.Availability7d != 50 {
		t.Fatalf("fallback = %s %.2f, want failed 50", monitor.PrimaryStatus, monitor.Availability7d)
	}
	if len(monitor.Timeline) != 2 {
		t.Fatalf("timeline = %d, want 2", len(monitor.Timeline))
	}
}

func TestAppendTrafficAvailabilityMonitorsPreservesExplicitMonitor(t *testing.T) {
	tracker := NewModelAvailabilityTracker()
	tracker.Record("veo-3", false)

	groups := []PublicTransitGroup{{
		Models: []PublicTransitModel{{
			StandardModel: "veo-3",
			BillingMode:   string(BillingModeVideo),
		}},
	}}
	explicit := []PublicTransitMonitor{{PrimaryModel: "veo-3", PrimaryStatus: "operational"}}
	monitors := appendTrafficAvailabilityMonitorsWithTracker(groups, explicit, tracker)
	if len(monitors) != 1 || monitors[0].PrimaryStatus != "operational" {
		t.Fatalf("explicit monitor was replaced: %#v", monitors)
	}
}

func TestAppendTrafficAvailabilityMonitorsSkipsTextModels(t *testing.T) {
	tracker := NewModelAvailabilityTracker()
	tracker.Record("gpt-5", true)

	groups := []PublicTransitGroup{{
		Models: []PublicTransitModel{{StandardModel: "gpt-5", BillingMode: string(BillingModeToken)}},
	}}
	if monitors := appendTrafficAvailabilityMonitorsWithTracker(groups, nil, tracker); len(monitors) != 0 {
		t.Fatalf("text model produced %d fallback monitors", len(monitors))
	}
}

func TestIsPublicMediaModelRecognizesConfiguredAndRegisteredFamilies(t *testing.T) {
	cases := []PublicTransitModel{
		{StandardModel: "gemini-3-pro-image", BillingMode: string(BillingModeToken)},
		{StandardModel: "happyhorse-1.1-t2v", BillingMode: string(BillingModeToken)},
		{StandardModel: "custom-image-endpoint", BillingMode: string(BillingModeToken), Price: &PublicTransitModelPrice{ImageSizePrices: map[string]*float64{"1k": testAvailabilityFloatPtr(1)}}},
	}
	for _, model := range cases {
		if !isPublicMediaModel(model) {
			t.Fatalf("model %q was not recognized as media", model.StandardModel)
		}
	}
}

func testAvailabilityFloatPtr(value float64) *float64 {
	return &value
}

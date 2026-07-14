package redline

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestShouldCaptureBelowThreshold(t *testing.T) {
	r, _ := New(Config{Service: "s", Registry: prometheus.NewRegistry(), Sampling: SamplingConfig{FullCaptureRPS: 10, OneInN: 1000}})
	t.Cleanup(r.Close)
	// At or below FullCaptureRPS every event is captured regardless of OneInN.
	for _, rate := range []uint64{0, 5, 10} {
		r.lastRate.Store(rate)
		if !r.shouldCapture() {
			t.Fatalf("rate=%d: expected full capture at/below threshold", rate)
		}
	}
}

func TestShouldCaptureAboveThresholdOneInOne(t *testing.T) {
	r, _ := New(Config{Service: "s", Registry: prometheus.NewRegistry(), Sampling: SamplingConfig{FullCaptureRPS: 10, OneInN: 1}})
	t.Cleanup(r.Close)
	r.lastRate.Store(1000)
	// OneInN==1 means rand.Uint64N(1) is always 0, so capture is deterministic.
	for i := 0; i < 100; i++ {
		if !r.shouldCapture() {
			t.Fatal("OneInN=1 must always capture")
		}
	}
}

func TestShouldCaptureAboveThresholdSamples(t *testing.T) {
	r, _ := New(Config{Service: "s", Registry: prometheus.NewRegistry(), Sampling: SamplingConfig{FullCaptureRPS: 10, OneInN: 5}})
	t.Cleanup(r.Close)
	r.lastRate.Store(1000)
	captured := 0
	const n = 20000
	for i := 0; i < n; i++ {
		if r.shouldCapture() {
			captured++
		}
	}
	// Expect roughly 1/5 above the threshold; assert it is throttled well below
	// full capture and clearly above zero (loose bounds to avoid flakiness).
	if captured >= n/2 || captured == 0 {
		t.Fatalf("captured=%d/%d, expected roughly 1/5 throttling", captured, n)
	}
}

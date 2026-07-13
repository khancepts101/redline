package main

import (
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func mkResponse(counts map[string]int) response {
	var v response
	for name, c := range counts {
		v.Data.SeriesCountByMetricName = append(v.Data.SeriesCountByMetricName, struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}{name, c})
	}
	return v
}

// gather runs the collector through a real registry (exercising Describe/Collect)
// and returns metric name -> label value -> gauge value.
func gather(t *testing.T, g *guard) map[string]map[string]float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(g)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]map[string]float64{}
	for _, mf := range mfs {
		vals := map[string]float64{}
		for _, m := range mf.Metric {
			vals[m.Label[0].GetValue()] = m.Gauge.GetValue()
		}
		out[mf.GetName()] = vals
	}
	return out
}

func TestUpdateThresholds(t *testing.T) {
	g := newGuard(50, 500)
	breaches := g.update(mkResponse(map[string]int{"low": 5, "warn_metric": 100, "hard_metric": 1000}))

	joined := strings.Join(breaches, "\n")
	if !strings.Contains(joined, "HARD metric=hard_metric") {
		t.Errorf("missing hard breach: %v", breaches)
	}
	if !strings.Contains(joined, "WARN metric=warn_metric") {
		t.Errorf("missing warn breach: %v", breaches)
	}
	if strings.Contains(joined, "low") {
		t.Errorf("unexpected breach for below-warn metric: %v", breaches)
	}
	if v := gather(t, g)["cardinality_guard_series_total"]["hard_metric"]; v != 1000 {
		t.Errorf("series gauge=%v, want 1000", v)
	}
}

func TestUpdateGrowth(t *testing.T) {
	g := newGuard(50000, 100000)
	g.update(mkResponse(map[string]int{"m": 100}))
	if v := gather(t, g)["cardinality_guard_series_growth"]["m"]; v != 100 {
		t.Fatalf("first growth=%v, want 100 (from zero baseline)", v)
	}
	g.update(mkResponse(map[string]int{"m": 130}))
	if v := gather(t, g)["cardinality_guard_series_growth"]["m"]; v != 30 {
		t.Fatalf("second growth=%v, want 30", v)
	}
}

// A metric that disappears from the TSDB response must stop being reported,
// rather than lingering at its last-seen value (the tool's own stale-series bug).
func TestUpdatePrunesStaleMetrics(t *testing.T) {
	g := newGuard(50000, 100000)
	g.update(mkResponse(map[string]int{"gone": 42}))
	g.update(mkResponse(map[string]int{"present": 7}))

	if got := testutil.CollectAndCount(g, "cardinality_guard_series_total"); got != 1 {
		t.Fatalf("series has %d entries, want 1 (stale metric not pruned)", got)
	}
	series := gather(t, g)["cardinality_guard_series_total"]
	if _, ok := series["gone"]; ok {
		t.Fatal("stale metric still reported after it left the poll")
	}
	if series["present"] != 7 {
		t.Fatalf("present=%v, want 7", series["present"])
	}
	if _, ok := g.previous["gone"]; ok {
		t.Fatal("previous still tracks a metric absent from the latest poll")
	}
}

// Concurrent scrapes during a poll must always see a whole snapshot, never a
// half-rebuilt one. Under -race this also guards the atomic handoff.
func TestCollectDuringUpdateIsConsistent(t *testing.T) {
	g := newGuard(50000, 100000)
	g.update(mkResponse(map[string]int{"a": 1, "b": 2, "c": 3}))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				g.update(mkResponse(map[string]int{"a": i, "b": i, "c": i}))
			}
		}
	}()

	for i := 0; i < 2000; i++ {
		// Each snapshot always has exactly three series; a torn read would not.
		if got := testutil.CollectAndCount(g, "cardinality_guard_series_total"); got != 3 {
			close(stop)
			wg.Wait()
			t.Fatalf("scrape saw %d series, want 3 (torn snapshot)", got)
		}
	}
	close(stop)
	wg.Wait()
}

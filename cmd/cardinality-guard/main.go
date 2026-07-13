package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type response struct {
	Status string `json:"status"`
	Data   struct {
		SeriesCountByMetricName []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		} `json:"seriesCountByMetricName"`
	} `json:"data"`
}

// guard is a prometheus.Collector that serves each scrape from an immutable
// snapshot swapped in atomically after every poll. Collect never mutates state,
// so a scrape can never observe a half-rebuilt metric set (the transient a
// GaugeVec.Reset() + repopulate would expose).
type guard struct {
	warn, hard int
	seriesDesc *prometheus.Desc
	growthDesc *prometheus.Desc

	snap     atomic.Pointer[snapshot]
	previous map[string]int // owned solely by the polling goroutine; no lock needed
}

type metricState struct{ value, growth int }
type snapshot map[string]metricState

func newGuard(warn, hard int) *guard {
	return &guard{
		warn:       warn,
		hard:       hard,
		seriesDesc: prometheus.NewDesc("cardinality_guard_series_total", "Active series by metric name.", []string{"metric"}, nil),
		growthDesc: prometheus.NewDesc("cardinality_guard_series_growth", "Series change since previous poll.", []string{"metric"}, nil),
		previous:   map[string]int{},
	}
}

func (g *guard) Describe(ch chan<- *prometheus.Desc) {
	ch <- g.seriesDesc
	ch <- g.growthDesc
}

func (g *guard) Collect(ch chan<- prometheus.Metric) {
	snap := g.snap.Load()
	if snap == nil {
		return
	}
	for name, s := range *snap {
		ch <- prometheus.MustNewConstMetric(g.seriesDesc, prometheus.GaugeValue, float64(s.value), name)
		ch <- prometheus.MustNewConstMetric(g.growthDesc, prometheus.GaugeValue, float64(s.growth), name)
	}
}

// update builds a fresh snapshot from a TSDB status response, publishes it
// atomically, and returns human-readable breach lines for warn/hard thresholds.
// Metric names absent from the response drop out of the new snapshot (and out of
// previous), so stale series stop being reported.
func (g *guard) update(v response) []string {
	sort.Slice(v.Data.SeriesCountByMetricName, func(i, j int) bool {
		return v.Data.SeriesCountByMetricName[i].Value > v.Data.SeriesCountByMetricName[j].Value
	})
	next := make(snapshot, len(v.Data.SeriesCountByMetricName))
	var breaches []string
	for _, m := range v.Data.SeriesCountByMetricName {
		next[m.Name] = metricState{value: m.Value, growth: m.Value - g.previous[m.Name]}
		g.previous[m.Name] = m.Value
		if m.Value >= g.hard {
			breaches = append(breaches, fmt.Sprintf("HARD metric=%s series=%d; consider metric_relabel_configs drop", m.Name, m.Value))
		} else if m.Value >= g.warn {
			breaches = append(breaches, fmt.Sprintf("WARN metric=%s series=%d", m.Name, m.Value))
		}
	}
	for name := range g.previous {
		if _, ok := next[name]; !ok {
			delete(g.previous, name)
		}
	}
	g.snap.Store(&next)
	return breaches
}

func main() {
	prom := flag.String("prometheus", "http://prometheus:9090", "Prometheus URL")
	listen := flag.String("listen", ":9091", "metrics address")
	warn := flag.Int("warn", 10000, "warning series count")
	hard := flag.Int("hard", 50000, "hard series count")
	interval := flag.Duration("interval", 30*time.Second, "poll interval")
	flag.Parse()
	g := newGuard(*warn, *hard)
	prometheus.MustRegister(g)
	go func() { http.Handle("/metrics", promhttp.Handler()); log.Fatal(http.ListenAndServe(*listen, nil)) }()
	for {
		resp, err := http.Get(*prom + "/api/v1/status/tsdb?limit=10000")
		if err == nil {
			var v response
			err = json.NewDecoder(resp.Body).Decode(&v)
			resp.Body.Close()
			if err == nil {
				for _, line := range g.update(v) {
					log.Print(line)
				}
			}
		}
		if err != nil {
			fmt.Printf("poll failed: %v\n", err)
		}
		time.Sleep(*interval)
	}
}

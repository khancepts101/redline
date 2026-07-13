package redline

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"time"
)

type JobRunner struct {
	r        *Redline
	name     string
	fn       func(context.Context) error
	registry *prometheus.Registry
	runs     prometheus.Counter
	failures prometheus.Counter
	duration prometheus.Histogram
	last     prometheus.Gauge
}

func (r *Redline) Job(name string, fn func(context.Context) error) *JobRunner {
	reg := prometheus.NewRegistry()
	j := &JobRunner{r: r, name: name, fn: fn, registry: reg, runs: prometheus.NewCounter(prometheus.CounterOpts{Name: "redline_job_runs_total", Help: "Completed job runs."}), failures: prometheus.NewCounter(prometheus.CounterOpts{Name: "redline_job_failures_total", Help: "Failed job runs."}), duration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "redline_job_duration_seconds", Help: "Job duration."}), last: prometheus.NewGauge(prometheus.GaugeOpts{Name: "redline_job_last_run_timestamp_seconds", Help: "Unix time of last completion; alert on staleness."})}
	reg.MustRegister(j.runs, j.failures, j.duration, j.last)
	return j
}
func (j *JobRunner) Run(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		if v := recover(); v != nil {
			j.r.capture(ctx, v)
			err = errors.New("job panicked")
			if j.r.cfg.PanicMode == PanicRepanic {
				// Record the run before the re-panic unwinds the defer, so the
				// pushed metrics reflect this failure. runs>=failures holds.
				j.runs.Inc()
				j.failures.Inc()
				j.duration.Observe(time.Since(start).Seconds())
				j.last.SetToCurrentTime()
				j.push()
				panic(v)
			}
		}
		j.runs.Inc()
		if err != nil {
			j.failures.Inc()
		}
		j.duration.Observe(time.Since(start).Seconds())
		j.last.SetToCurrentTime()
		if e := j.push(); err == nil {
			err = e
		}
	}()
	return j.fn(withLogger(ctx, j.r.cfg.Logger))
}
func (j *JobRunner) push() error {
	if j.r.cfg.PushgatewayURL == "" {
		return nil
	}
	return push.New(j.r.cfg.PushgatewayURL, j.name).Gatherer(j.registry).Grouping("service", j.r.cfg.Service).Push()
}
func (j *JobRunner) Delete() error {
	if j.r.cfg.PushgatewayURL == "" {
		return nil
	}
	return push.New(j.r.cfg.PushgatewayURL, j.name).Grouping("service", j.r.cfg.Service).Delete()
}

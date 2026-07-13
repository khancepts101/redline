// Package redline instruments HTTP and gRPC services with RED metrics and error capture.
package redline

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

type PanicMode int

const (
	PanicRepanic PanicMode = iota
	PanicRespond500
)

type Config struct {
	Service        string
	Registry       prometheus.Registerer
	DSN            string
	PanicMode      PanicMode
	Logger         *slog.Logger
	Buckets        []float64
	Sampling       SamplingConfig
	PushgatewayURL string
}

type SamplingConfig struct {
	FullCaptureRPS float64
	OneInN         uint64
}

type Redline struct {
	cfg      Config
	requests *prometheus.CounterVec
	errors   *prometheus.CounterVec
	duration *prometheus.HistogramVec
	overhead *prometheus.HistogramVec
	rate     atomic.Uint64
	lastRate atomic.Uint64
	sentry   *sentry.Client
}

func New(cfg Config) (*Redline, error) {
	if cfg.Service == "" {
		cfg.Service = "unknown"
	}
	if cfg.Registry == nil {
		cfg.Registry = prometheus.DefaultRegisterer
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.Buckets) == 0 {
		cfg.Buckets = prometheus.DefBuckets
	}
	if cfg.Sampling.FullCaptureRPS == 0 {
		cfg.Sampling.FullCaptureRPS = 10
	}
	if cfg.Sampling.OneInN == 0 {
		cfg.Sampling.OneInN = 10
	}
	r := &Redline{cfg: cfg}
	r.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "redline_requests_total", Help: "HTTP and RPC requests."}, []string{"service", "transport", "method", "route", "code"})
	r.errors = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "redline_errors_total", Help: "Errors classified by type."}, []string{"service", "transport", "method", "route", "class"})
	r.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "redline_request_duration_seconds", Help: "Request duration with trace exemplars.", Buckets: cfg.Buckets}, []string{"service", "transport", "method", "route"})
	r.overhead = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "redline_middleware_overhead_seconds", Help: "Time spent by redline after the wrapped operation.", Buckets: prometheus.ExponentialBuckets(0.000001, 2, 14)}, []string{"service", "transport"})
	for _, c := range []prometheus.Collector{r.requests, r.errors, r.duration, r.overhead} {
		if err := cfg.Registry.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return nil, err
			}
		}
	}
	if cfg.DSN != "" {
		client, err := sentry.NewClient(sentry.ClientOptions{Dsn: cfg.DSN, AttachStacktrace: true})
		if err != nil {
			return nil, err
		}
		r.sentry = client
	}
	go r.measureRate()
	return r, nil
}

func (r *Redline) measureRate() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		r.lastRate.Store(r.rate.Swap(0))
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	// Only the first status reaches the client (net/http drops the rest), so a
	// later WriteHeader — e.g. http.Error(500) after a handler already responded
	// then panicked — must not rewrite the recorded code.
	if w.code != 0 {
		return
	}
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// HTTP wraps a handler. Pass a stable route template (for example /users/{id}), never a raw URL.
func (r *Redline) HTTP(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		r.rate.Add(1)
		sw := &statusWriter{ResponseWriter: w}
		ctx := withLogger(req.Context(), r.cfg.Logger)
		req = req.WithContext(ctx)
		panicked := false
		var panicValue any
		defer func() {
			if v := recover(); v != nil {
				panicked = true
				panicValue = v
				r.errors.WithLabelValues(r.cfg.Service, "http", req.Method, route, "panic").Inc()
				r.capture(ctx, v)
				if r.cfg.PanicMode == PanicRespond500 {
					http.Error(sw, "internal server error", 500)
				}
			}
			code := sw.code
			if code == 0 {
				// A re-panicked request never wrote a status; account for it as
				// a 500 rather than a phantom 200.
				if panicked {
					code = 500
				} else {
					code = 200
				}
			}
			after := time.Now()
			r.requests.WithLabelValues(r.cfg.Service, "http", req.Method, route, strconv.Itoa(code)).Inc()
			// A panic is already counted in the "panic" class; don't also count
			// the resulting 500 as "5xx" or one request would inflate the
			// error-rate ratio (numerator 2, denominator 1).
			if code >= 500 && !panicked {
				r.errors.WithLabelValues(r.cfg.Service, "http", req.Method, route, "5xx").Inc()
			}
			r.observe(r.duration.WithLabelValues(r.cfg.Service, "http", req.Method, route), after.Sub(start).Seconds(), ctx)
			r.overhead.WithLabelValues(r.cfg.Service, "http").Observe(time.Since(after).Seconds())
			// Metrics are finalized above; only now re-raise so the request is
			// still accounted for (matches the job runner's ordering).
			if panicked && r.cfg.PanicMode == PanicRepanic {
				panic(panicValue)
			}
		}()
		next.ServeHTTP(sw, req)
	})
}

func (r *Redline) observe(o prometheus.Observer, seconds float64, ctx context.Context) {
	if eo, ok := o.(prometheus.ExemplarObserver); ok {
		if id := TraceID(ctx); id != "" {
			eo.ObserveWithExemplar(seconds, prometheus.Labels{"trace_id": id})
			return
		}
	}
	o.Observe(seconds)
}
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}
func (r *Redline) shouldCapture() bool {
	return float64(r.lastRate.Load()) <= r.cfg.Sampling.FullCaptureRPS || rand.Uint64N(r.cfg.Sampling.OneInN) == 0
}
func (r *Redline) capture(ctx context.Context, value any) {
	if r.sentry == nil || !r.shouldCapture() {
		return
	}
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.Message = "panic"
	event.Exception = []sentry.Exception{{Value: toString(value), Stacktrace: sentry.NewStacktrace()}}
	if id := TraceID(ctx); id != "" {
		event.Tags["trace_id"] = id
	}
	r.sentry.CaptureEvent(event, nil, nil)
}
func toString(v any) string {
	if e, ok := v.(error); ok {
		return e.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return "non-string panic"
}

type loggerKey struct{}

func withLogger(ctx context.Context, l *slog.Logger) context.Context {
	if id := TraceID(ctx); id != "" {
		l = l.With("trace_id", id)
	}
	return context.WithValue(ctx, loggerKey{}, l)
}
func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

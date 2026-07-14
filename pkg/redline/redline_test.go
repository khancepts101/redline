package redline

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"net/http"
	"net/http/httptest"
	"testing"
)

// histCount returns how many observations a histogram (from a HistogramVec's
// WithLabelValues) has recorded.
func histCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	m, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer %T is not a prometheus.Metric", o)
	}
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatal(err)
	}
	return dm.GetHistogram().GetSampleCount()
}

func TestHTTP(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, err := New(Config{Service: "test", Registry: reg, PanicMode: PanicRespond500})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	h := r.HTTP("/ok", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "http", "GET", "/ok", "204")); n != 1 {
		t.Fatalf("requests=%v", n)
	}
}
func TestPanicResponds500(t *testing.T) {
	r, _ := New(Config{Service: "panic", Registry: prometheus.NewRegistry(), PanicMode: PanicRespond500})
	t.Cleanup(r.Close)
	w := httptest.NewRecorder()
	r.HTTP("/panic", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).ServeHTTP(w, httptest.NewRequest("GET", "/panic", nil))
	if w.Code != 500 {
		t.Fatalf("code=%d", w.Code)
	}
}

// In PanicRepanic mode the panic must propagate, but request/duration/overhead
// must still be recorded first (regression: re-panic exited the defer early).
func TestHTTPPanicRepanicRecordsMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, _ := New(Config{Service: "repanic", Registry: reg, PanicMode: PanicRepanic})
	t.Cleanup(r.Close)
	h := r.HTTP("/panic", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected re-panic")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/panic", nil))
	}()
	if n := testutil.ToFloat64(r.requests.WithLabelValues("repanic", "http", "GET", "/panic", "500")); n != 1 {
		t.Fatalf("requests{code=500}=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("repanic", "http", "GET", "/panic", "panic")); n != 1 {
		t.Fatalf("errors{panic}=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("repanic", "http", "GET", "/panic", "5xx")); n != 0 {
		t.Fatalf("errors{5xx}=%v, want 0 (panic already counted)", n)
	}
	if n := histCount(t, r.duration.WithLabelValues("repanic", "http", "GET", "/panic")); n != 1 {
		t.Fatalf("duration observations=%v, want 1", n)
	}
	if n := histCount(t, r.overhead.WithLabelValues("repanic", "http")); n != 1 {
		t.Fatalf("overhead observations=%v, want 1", n)
	}
}

// A handler that writes a status and then panics must keep the status the client
// actually received; the later http.Error(500) cannot change it.
func TestHTTPStatusPreservedOnPanicAfterWrite(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, _ := New(Config{Service: "svc", Registry: reg, PanicMode: PanicRespond500})
	t.Cleanup(r.Close)
	h := r.HTTP("/x", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(202)
		panic("after write")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != 202 {
		t.Fatalf("client got code=%d, want 202", w.Code)
	}
	if n := testutil.ToFloat64(r.requests.WithLabelValues("svc", "http", "GET", "/x", "202")); n != 1 {
		t.Fatalf("requests{202}=%v, want 1 (metric must match delivered status)", n)
	}
	if n := testutil.ToFloat64(r.requests.WithLabelValues("svc", "http", "GET", "/x", "500")); n != 0 {
		t.Fatalf("requests{500}=%v, want 0", n)
	}
}

func TestCloseStopsRateSamplerAndIsIdempotent(t *testing.T) {
	r, err := New(Config{Service: "close", Registry: prometheus.NewRegistry()})
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	select {
	case <-r.done:
	default:
		t.Fatal("rate sampler did not stop")
	}
	r.Close()
}

func TestToStringPreservesPanicValue(t *testing.T) {
	if got := toString(42); got != "42" {
		t.Fatalf("toString(42)=%q, want 42", got)
	}
}

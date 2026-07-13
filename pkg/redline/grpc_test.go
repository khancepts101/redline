package redline

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newRedline(t *testing.T, mode PanicMode) *Redline {
	t.Helper()
	r, err := New(Config{Service: "test", Registry: prometheus.NewRegistry(), PanicMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func invokeUnary(r *Redline, method string, handler grpc.UnaryHandler) (any, error) {
	interceptor := r.UnaryServerInterceptor()
	return interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
}

func TestUnaryOK(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	_, err := invokeUnary(r, "/svc/OK", func(context.Context, any) (any, error) { return "ok", nil })
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "unary", "/svc/OK", "/svc/OK", "OK")); n != 1 {
		t.Fatalf("requests=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "unary", "/svc/OK", "/svc/OK", "OK")); n != 0 {
		t.Fatalf("errors=%v, want 0", n)
	}
}

func TestUnaryError(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	_, err := invokeUnary(r, "/svc/Err", func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "nope")
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code=%v", status.Code(err))
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "unary", "/svc/Err", "/svc/Err", "NotFound")); n != 1 {
		t.Fatalf("errors=%v, want 1", n)
	}
}

func TestUnaryPanicRespondsInternal(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	_, err := invokeUnary(r, "/svc/Panic", func(context.Context, any) (any, error) { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("code=%v, want Internal", status.Code(err))
	}
	// One panicked call: one request and one error, no double count.
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "unary", "/svc/Panic", "/svc/Panic", "Internal")); n != 1 {
		t.Fatalf("requests=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "unary", "/svc/Panic", "/svc/Panic", "Internal")); n != 1 {
		t.Fatalf("errors=%v, want 1", n)
	}
}

// The gRPC interceptors must feed the request-rate counter so that adaptive
// capture sampling engages for gRPC-only services.
func TestUnaryFeedsRate(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	before := r.rate.Load()
	_, _ = invokeUnary(r, "/svc/OK", func(context.Context, any) (any, error) { return "ok", nil })
	if got := r.rate.Load(); got != before+1 {
		t.Fatalf("rate=%d, want %d", got, before+1)
	}
}

// In PanicRepanic mode the panic propagates, but request/duration must still be
// recorded first (regression: re-panic exited the defer before recordRPC).
func TestUnaryPanicRepanicRecordsMetrics(t *testing.T) {
	r := newRedline(t, PanicRepanic)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected re-panic")
			}
		}()
		_, _ = invokeUnary(r, "/svc/Panic", func(context.Context, any) (any, error) { panic("boom") })
	}()
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "unary", "/svc/Panic", "/svc/Panic", "Internal")); n != 1 {
		t.Fatalf("requests=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "unary", "/svc/Panic", "/svc/Panic", "Internal")); n != 1 {
		t.Fatalf("errors=%v, want 1", n)
	}
	if n := histCount(t, r.duration.WithLabelValues("test", "unary", "/svc/Panic", "/svc/Panic")); n != 1 {
		t.Fatalf("duration observations=%v, want 1", n)
	}
	if n := histCount(t, r.overhead.WithLabelValues("test", "unary")); n != 1 {
		t.Fatalf("overhead observations=%v, want 1", n)
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

func invokeStream(r *Redline, method string, handler grpc.StreamHandler) error {
	interceptor := r.StreamServerInterceptor()
	ss := &fakeStream{ctx: context.Background()}
	return interceptor(nil, ss, &grpc.StreamServerInfo{FullMethod: method}, handler)
}

func TestStreamOKAndFeedsRate(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	before := r.rate.Load()
	if err := invokeStream(r, "/svc/Stream", func(any, grpc.ServerStream) error { return nil }); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := r.rate.Load(); got != before+1 {
		t.Fatalf("rate=%d, want %d", got, before+1)
	}
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "stream", "/svc/Stream", "/svc/Stream", "OK")); n != 1 {
		t.Fatalf("requests=%v, want 1", n)
	}
}

func TestStreamPanicRespondsInternal(t *testing.T) {
	r := newRedline(t, PanicRespond500)
	err := invokeStream(r, "/svc/Stream", func(any, grpc.ServerStream) error { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("code=%v, want Internal", status.Code(err))
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "stream", "/svc/Stream", "/svc/Stream", "Internal")); n != 1 {
		t.Fatalf("errors=%v, want 1", n)
	}
}

func TestStreamPanicRepanicRecordsMetrics(t *testing.T) {
	r := newRedline(t, PanicRepanic)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected re-panic")
			}
		}()
		_ = invokeStream(r, "/svc/Stream", func(any, grpc.ServerStream) error { panic("boom") })
	}()
	if n := testutil.ToFloat64(r.requests.WithLabelValues("test", "stream", "/svc/Stream", "/svc/Stream", "Internal")); n != 1 {
		t.Fatalf("requests=%v, want 1", n)
	}
	if n := testutil.ToFloat64(r.errors.WithLabelValues("test", "stream", "/svc/Stream", "/svc/Stream", "Internal")); n != 1 {
		t.Fatalf("errors=%v, want 1", n)
	}
	if n := histCount(t, r.duration.WithLabelValues("test", "stream", "/svc/Stream", "/svc/Stream")); n != 1 {
		t.Fatalf("duration observations=%v, want 1", n)
	}
	if n := histCount(t, r.overhead.WithLabelValues("test", "stream")); n != 1 {
		t.Fatalf("overhead observations=%v, want 1", n)
	}
}

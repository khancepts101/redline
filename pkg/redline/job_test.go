package redline

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newJob(t *testing.T, mode PanicMode, fn func(context.Context) error) *JobRunner {
	t.Helper()
	r, err := New(Config{Service: "test", Registry: prometheus.NewRegistry(), PanicMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r.Job("test_job", fn)
}

func TestJobSuccess(t *testing.T) {
	j := newJob(t, PanicRespond500, func(context.Context) error { return nil })
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("err=%v", err)
	}
	if n := testutil.ToFloat64(j.runs); n != 1 {
		t.Fatalf("runs=%v, want 1", n)
	}
	if n := testutil.ToFloat64(j.failures); n != 0 {
		t.Fatalf("failures=%v, want 0", n)
	}
}

func TestJobError(t *testing.T) {
	j := newJob(t, PanicRespond500, func(context.Context) error { return errors.New("boom") })
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if n := testutil.ToFloat64(j.runs); n != 1 {
		t.Fatalf("runs=%v, want 1", n)
	}
	if n := testutil.ToFloat64(j.failures); n != 1 {
		t.Fatalf("failures=%v, want 1", n)
	}
}

// A panicking job in the non-repanic mode must count exactly one run and one
// failure, not two failures (regression: the failure was double-counted).
func TestJobPanicCountsOnce(t *testing.T) {
	j := newJob(t, PanicRespond500, func(context.Context) error { panic("boom") })
	err := j.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from panicked job")
	}
	if n := testutil.ToFloat64(j.runs); n != 1 {
		t.Fatalf("runs=%v, want 1", n)
	}
	if n := testutil.ToFloat64(j.failures); n != 1 {
		t.Fatalf("failures=%v, want 1", n)
	}
}

// In repanic mode the panic propagates, but metrics must still be recorded once
// and satisfy runs >= failures.
func TestJobPanicRepanicRecordsOnce(t *testing.T) {
	j := newJob(t, PanicRepanic, func(context.Context) error { panic("boom") })
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected re-panic")
			}
		}()
		_ = j.Run(context.Background())
	}()
	if n := testutil.ToFloat64(j.runs); n != 1 {
		t.Fatalf("runs=%v, want 1", n)
	}
	if n := testutil.ToFloat64(j.failures); n != 1 {
		t.Fatalf("failures=%v, want 1", n)
	}
}

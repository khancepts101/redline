package benchmarks

import (
	"github.com/khancepts101/redline/pkg/redline"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/http/httptest"
	"testing"
)

var bare = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })

func BenchmarkBare(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bare.ServeHTTP(httptest.NewRecorder(), req)
	}
}
func BenchmarkRedline(b *testing.B) {
	r, _ := redline.New(redline.Config{Service: "bench", Registry: prometheus.NewRegistry()})
	h := r.HTTP("/", bare)
	req := httptest.NewRequest("GET", "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

package main

import (
	crand "crypto/rand"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/khancepts101/redline/pkg/redline"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
)

func withDemoTrace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var traceID trace.TraceID
		var spanID trace.SpanID
		if _, err := crand.Read(traceID[:]); err != nil {
			http.Error(w, "trace id generation failed", http.StatusInternalServerError)
			return
		}
		if _, err := crand.Read(spanID[:]); err != nil {
			http.Error(w, "span id generation failed", http.StatusInternalServerError)
			return
		}
		sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true})
		w.Header().Set("X-Trace-ID", traceID.String())
		next.ServeHTTP(w, req.WithContext(trace.ContextWithRemoteSpanContext(req.Context(), sc)))
	})
}

func main() {
	r, err := redline.New(redline.Config{Service: "demo", DSN: os.Getenv("GLITCHTIP_DSN"), PanicMode: redline.PanicRespond500})
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	mux.Handle("/api/work", withDemoTrace(r.HTTP("/api/work", http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		time.Sleep(time.Duration(rand.IntN(80)) * time.Millisecond)
		if q.URL.Query().Get("panic") == "1" {
			panic("injected panic")
		}
		if q.URL.Query().Get("error") == "1" {
			http.Error(w, "injected failure", 500)
			return
		}
		w.Write([]byte("ok\n"))
	}))))
	log.Println("demo listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

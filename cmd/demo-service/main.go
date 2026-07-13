package main

import (
	"github.com/khancepts101/redline/pkg/redline"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"time"
)

func main() {
	r, err := redline.New(redline.Config{Service: "demo", DSN: os.Getenv("GLITCHTIP_DSN"), PanicMode: redline.PanicRespond500})
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/api/work", r.HTTP("/api/work", http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		time.Sleep(time.Duration(rand.IntN(80)) * time.Millisecond)
		if q.URL.Query().Get("panic") == "1" {
			panic("injected panic")
		}
		if q.URL.Query().Get("error") == "1" {
			http.Error(w, "injected failure", 500)
			return
		}
		w.Write([]byte("ok\n"))
	})))
	log.Println("demo listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

package main

import (
	"flag"
	"log"
	"math/rand/v2"
	"net/http"
	"time"
)

func main() {
	url := flag.String("url", "http://demo:8080/api/work", "target")
	rps := flag.Int("rps", 5, "requests per second")
	errors := flag.Float64("error-rate", .08, "failure probability")
	flag.Parse()
	t := time.NewTicker(time.Second / time.Duration(*rps))
	defer t.Stop()
	for range t.C {
		u := *url
		if rand.Float64() < *errors {
			u += "?error=1"
		}
		resp, err := http.Get(u)
		if err != nil {
			log.Print(err)
			continue
		}
		resp.Body.Close()
	}
}

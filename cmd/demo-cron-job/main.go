package main

import (
	"context"
	"github.com/khancepts101/redline/pkg/redline"
	"log"
	"os"
	"time"
)

func main() {
	r, err := redline.New(redline.Config{Service: "demo-cron", PushgatewayURL: os.Getenv("PUSHGATEWAY_URL"), DSN: os.Getenv("GLITCHTIP_DSN"), PanicMode: redline.PanicRespond500})
	if err != nil {
		log.Fatal(err)
	}
	job := r.Job("demo_cron", func(context.Context) error { time.Sleep(200 * time.Millisecond); return nil })
	for {
		if err := job.Run(context.Background()); err != nil {
			log.Print(err)
		}
		time.Sleep(time.Minute)
	}
}

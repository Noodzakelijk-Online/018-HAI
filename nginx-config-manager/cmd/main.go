package main

import (
	"automation-hub-nginxconfigmanager/internal/app/autoconfig"
	"automation-hub-nginxconfigmanager/internal/app/config"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config.Init()

	consumer, err := autoconfig.DefaultConsumer()
	if err != nil {
		return err
	}
	defer consumer.Close()

	server := &http.Server{
		Addr:              ":" + config.AppConfig.HealthPort,
		Handler:           healthHandler(consumer.Ready),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	errs := make(chan error, 2)
	go func() {
		log.Printf("Starting health server on port %s", config.AppConfig.HealthPort)
		errs <- server.ListenAndServe()
	}()
	go func() {
		log.Println("Starting Kafka consumer...")
		errs <- consumer.Start()
	}()

	return fmt.Errorf("nginx config manager stopped: %w", <-errs)
}

func healthHandler(ready func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"service": "nginx-config-manager",
				"status":  "not_ready",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "nginx-config-manager",
			"status":  "ok",
		})
	})
}

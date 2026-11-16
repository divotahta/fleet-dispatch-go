package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"fleet-dispatch-go/internal/handler"
	"fleet-dispatch-go/internal/repository"
	"fleet-dispatch-go/internal/service"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	telemetryCh := make(chan handler.TelemetryEvent, 64)
	var wg sync.WaitGroup

	// Worker goroutine membaca update GPS dari channel sampai server dimatikan
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range telemetryCh {
			logger.Info("telemetry gps diterima", "driver_id", ev.DriverID, "lat", ev.Lat, "lng", ev.Lng, "status", ev.Status)
		}
	}()

	repo := repository.NewMemoryRepository()
	svc := service.NewDispatchService(repo)
	h := handler.NewHandler(svc, logger, telemetryCh)

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("fleet-dispatch-go berjalan", "addr", "http://localhost:"+port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("memulai graceful shutdown...")

	graceCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(graceCtx); err != nil {
		// Jangan close telemetryCh jika masih ada handler aktif yang melewati batas shutdown
		logger.Error("graceful shutdown melewati batas waktu", "error", err)
		os.Exit(1)
	}

	close(telemetryCh)
	wg.Wait()
	logger.Info("server berhenti dengan bersih")
}

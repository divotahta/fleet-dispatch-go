package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"fleet-dispatch-go/internal/handler"
	"fleet-dispatch-go/internal/repository"
	"fleet-dispatch-go/internal/service"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loadDotEnv(".env")

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

	repo, cleanupDB, err := initRepository(logger)
	if err != nil {
		logger.Error("gagal menyiapkan repository", "error", err)
		os.Exit(1)
	}
	defer cleanupDB()

	svc := service.NewDispatchService(repo)
	h := handler.NewHandler(svc, logger, telemetryCh)

	port := strings.TrimSpace(os.Getenv("APP_PORT"))
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

func initRepository(logger *slog.Logger) (repository.FleetRepository, func(), error) {
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dsn == "" {
		logger.Info("DB_DSN kosong, menggunakan MemoryRepository (in-memory)")
		return repository.NewMemoryRepository(), func() {}, nil
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	logger.Info("terhubung ke MySQL, menggunakan MySQLRepository")
	return repository.NewMySQLRepository(db), func() { _ = db.Close() }, nil
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

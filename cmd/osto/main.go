package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osto/securecli/internal/auth"
	"github.com/osto/securecli/internal/cli"
	"github.com/osto/securecli/internal/db"
)

func main() {
	databaseURL := getenv("DATABASE_URL", "postgres://osto:osto-dev-password@localhost:55432/osto?sslmode=disable")
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database unavailable: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	config := auth.Config{
		SessionTimeout:   durationEnv("SESSION_TIMEOUT", 30*time.Minute),
		LockoutThreshold: intEnv("LOCKOUT_THRESHOLD", 5),
		LockoutDuration:  durationEnv("LOCKOUT_DURATION", 15*time.Minute),
	}
	service := auth.NewService(pool, config)
	if _, err := tea.NewProgram(cli.New(service, config.SessionTimeout), tea.WithAltScreen()).Run(); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

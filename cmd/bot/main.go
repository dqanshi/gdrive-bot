package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gdrive-bot/internal/bot"
	"gdrive-bot/internal/config"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("gdrive-bot starting")

	// Prefer config/.env; fall back to a bare .env in the working dir
	// for local development. Real VPS deployments use either the file or
	// environment variables injected by systemd.
	envPath := "config/.env"
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envPath = ".env"
	}

	cfg, err := config.Load(envPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	if err := bot.Run(ctx, cfg); err != nil {
		log.Fatalf("bot: %v", err)
	}
	log.Println("gdrive-bot stopped")
}

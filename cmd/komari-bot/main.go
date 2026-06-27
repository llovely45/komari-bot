package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"komari-bot/internal/app"
	"komari-bot/internal/config"
	"komari-bot/internal/currency"
	"komari-bot/internal/komari"
	"komari-bot/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Init(); err != nil {
		log.Fatalf("init database: %v", err)
	}

	komariClient := komari.NewClient(cfg.KomariURL, cfg.KomariKey)
	converter := currency.NewConverter(cfg.FXAPIURL)
	botApp, err := app.New(cfg, db, komariClient, converter)
	if err != nil {
		log.Fatalf("create bot app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := botApp.Run(ctx); err != nil {
		log.Fatalf("run bot: %v", err)
	}
}

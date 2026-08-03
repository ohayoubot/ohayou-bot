package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	_ "time/tzdata"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/game"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/catfact"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/deerkins"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/drop"
	"github.com/ohayoubot/ohayou-bot/internal/seed"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to the JSON config file")
	dataDir := flag.String("data", "data", "directory holding items.json and fortunes.txt")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(*configPath, *dataDir, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath, dataDir string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Init(ctx); err != nil {
		return err
	}

	// Sync the item catalog from items.json (inserts new items, updates prices
	// and other fields on existing ones) and load fortunes.
	items, err := seed.LoadItems(filepath.Join(dataDir, "items.json"))
	if err != nil {
		return err
	}
	if n, err := db.SeedItems(ctx, items); err != nil {
		return err
	} else if n > 0 {
		log.Info("synced item catalog", "items", n)
	}

	fortunes, err := seed.LoadFortunes(filepath.Join(dataDir, "fortunes.txt"))
	if err != nil {
		return err
	}
	log.Info("loaded fortunes", "count", len(fortunes))

	b := bot.New(cfg, log)
	catfact.New(b).Register()

	if cfg.Deerkins.Use() {
		deerkins.New(b, cfg.Deerkins).Register()
		log.Info("deerkins enabled", "database", cfg.Deerkins.DatabaseID, "editor", cfg.Deerkins.Editor)
	}

	var drops *drop.Plugin
	if cfg.Drop.Use() {
		drops = drop.New(b, cfg.Drop, db)
		drops.Register()
		drops.Start(ctx)
		log.Info("drop enabled", "database", cfg.Drop.DatabaseID, "url", cfg.Drop.URL)
	}

	g, err := game.New(b, db, fortunes, log)
	if err != nil {
		return err
	}
	g.Register()
	g.Start(ctx)

	// Run blocks until the context is cancelled (SIGINT/SIGTERM) and the IRC
	// loop exits. Once it returns the base context is already cancelled, so the
	// game's background goroutines are unblocking; wait for them to finish their
	// final store writes before the deferred db.Close runs.
	err = b.Run(ctx)
	g.Wait()
	if drops != nil {
		drops.Wait()
	}
	return err
}

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/catfact"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/deerkins"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/drop"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/youtube"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
	"github.com/ohayoubot/ohayou-bot/internal/task"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to the JSON config file")
	check := flag.Bool("check", false, "load the config, report what would run, and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	run := run
	if *check {
		run = validate
	}
	if err := run(*configPath, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// validate walks the same path startup does, as far as it can go without
// touching the network or the store: the config file, the environment behind
// it, and every plugin's own configuration. Those are what a bad deployment
// fails on, and failing here costs one exit code rather than a restart loop.
func validate(configPath string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	reg := plugin.NewRegistry(plugin.Deps{Log: log})
	reg.Add(plugins()...)
	if err := reg.Configure(cfg); err != nil {
		return err
	}

	log.Info("config is usable",
		"file", configPath,
		"nick", cfg.Nick,
		"server", cfg.Server,
		"channels", len(cfg.Channels),
		"plugins", strings.Join(reg.Names(), " "),
		"website", cfg.Web.URL != "" && cfg.Web.Secret != "")
	return nil
}

func run(configPath string, log *slog.Logger) error {
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

	b := bot.New(cfg, log)
	runner := task.NewRunner(db, b, log)

	minter := web.NewMinter(cfg.Web.Secret)
	publisher := web.NewPublisher(cfg.Web.URL, cfg.Web.Secret)
	reg := plugin.NewRegistry(plugin.Deps{
		Bot: b, Store: db, Log: log, Runner: runner,
		Web: minter, Publisher: publisher,
	})
	reg.Add(plugins()...)
	if err := reg.Configure(cfg); err != nil {
		return err
	}
	if err := reg.Register(); err != nil {
		return err
	}
	// After Configure, so the link carries what the enabled plugins asked for
	// and nothing from the ones that are off.
	if web.Install(b, minter, log, cfg.Web.URL, reg.Scopes()) {
		log.Info("website", "url", cfg.Web.URL, "scopes", reg.Scopes())
	}
	if err := reg.Start(ctx); err != nil {
		return err
	}
	// After the plugins, so every handler is claimed before anything is fired.
	if err := runner.Start(ctx); err != nil {
		return err
	}

	// Run blocks until the context is cancelled (SIGINT/SIGTERM) and the IRC
	// loop exits. Once it returns the base context is already cancelled, so the
	// tracked goroutines are unblocking; wait for them to finish their final
	// store writes before the deferred db.Close runs.
	err = b.Run(ctx)
	b.Wait()

	// The base context is cancelled by now, so the plugins get a fresh one to
	// write their final state with, while the store is still open.
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()
	reg.Stop(stopCtx)
	return err
}

// plugins is everything the bot can be asked to do, in the order they register.
// Which of them come up is the config's business, not this list's.
func plugins() []plugin.Plugin {
	return []plugin.Plugin{
		catfact.New(),
		deerkins.New(),
		youtube.New(),
		drop.New(),
		ohayou.New(),
	}
}

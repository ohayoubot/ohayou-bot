package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
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
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(*configPath, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
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

	reg := plugin.NewRegistry(plugin.Deps{
		Bot: b, Store: db, Log: log, Runner: runner,
		Web: web.NewMinter(cfg.Web.Secret),
	})
	reg.Add(plugins()...)
	if err := reg.Configure(cfg); err != nil {
		return err
	}
	if err := reg.Register(); err != nil {
		return err
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

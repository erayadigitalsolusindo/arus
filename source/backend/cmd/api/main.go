package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"aciraba/internal/audit"
	"aciraba/internal/platform/background"
	"aciraba/internal/platform/config"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/mailer"
	"aciraba/internal/platform/redisx"
)

// redisPinger menyesuaikan redis.Client ke httpx.Pinger.
type redisPinger struct{ c *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

func main() {
	if err := run(); err != nil {
		slog.Error("api berhenti dengan error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, nil)
	if cfg.IsDev() {
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	log := slog.New(handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb, err := redisx.Connect(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	r := httpx.NewRouter(log, cfg.CORSOrigins, cfg.TrustProxy)
	r.Use(audit.CaptureMeta) // IP + request id untuk audit log (middleware harus sebelum route)
	r.Get("/healthz", httpx.Healthz(map[string]httpx.Pinger{
		"postgres": pool,
		"redis":    redisPinger{rdb},
	}))

	mail, err := mailer.New(mailer.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.SMTPFrom}, log)
	if err != nil {
		return err
	}
	jobs := background.New(log, 16, 30*time.Second)
	defer jobs.Wait() // email yang sedang dikirim diselesaikan dulu saat shutdown

	if err := mountModules(r, appDeps{Ctx: ctx, Cfg: cfg, Log: log, Pool: pool, Redis: rdb, Mailer: mail, Jobs: jobs}); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api listen", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutdown")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
	return nil
}

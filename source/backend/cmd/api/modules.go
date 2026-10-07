package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/iam"
	"aciraba/internal/item"
	"aciraba/internal/outlet"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/background"
	"aciraba/internal/platform/config"
	"aciraba/internal/platform/mailer"
	"aciraba/internal/platformadmin"
)

// appDeps = infrastruktur bersama yang dirakit main() lalu diteruskan ke semua modul.
type appDeps struct {
	Cfg    config.Config
	Log    *slog.Logger
	Pool   *pgxpool.Pool
	Redis  *redis.Client
	Mailer mailer.Mailer
	Jobs   *background.Runner
}

// mountModules merakit tiap modul (layanan + handler) dan memasang rutenya. Modul baru = satu blok di sini.
// Urutan: authz (izin) → audit → modul yang memakainya.
func mountModules(r chi.Router, d appDeps) error {
	tokens := pauth.NewTokenIssuer(d.Cfg.JWTSecret)
	sessions := pauth.NewSessions(d.Redis)
	perms := authz.NewResolver(d.Pool)

	authSvc := auth.NewService(auth.Deps{
		Pool: d.Pool, Tokens: tokens, Sessions: sessions, OneTime: pauth.NewOneTime(d.Redis), Perms: perms,
		Mailer: d.Mailer, Jobs: d.Jobs, BaseURL: d.Cfg.AppBaseURL,
	})
	auth.NewHandler(auth.HandlerDeps{
		Service: authSvc, Log: d.Log, Redis: d.Redis,
		Lockout:    auth.NewLockout(d.Redis, auth.NewPolicyLoader(d.Pool, d.Log)),
		RateLimits: auth.NewRateLimitLoader(d.Pool, d.Log),
		Tokens:     tokens, Perms: perms, Origins: d.Cfg.CORSOrigins, SecureCookie: !d.Cfg.IsDev(),
	}).Routes(r)

	// Platform Admin (operator ACIRABA): token & cookie terpisah; lockout login memakai kebijakan yang sama.
	totpBox, err := pauth.NewTOTPBox(d.Cfg.JWTSecret)
	if err != nil {
		return err
	}
	platformSvc := platformadmin.NewService(platformadmin.Deps{
		Pool: d.Pool, Tokens: tokens, PTokens: pauth.NewPlatformTokenIssuer(d.Cfg.JWTSecret), Sessions: sessions,
		OneTime: pauth.NewOneTime(d.Redis), TOTP: totpBox, Perms: perms, SetupToken: d.Cfg.PlatformSetupToken,
	})
	perms.WithPlatform(platformSvc) // menerima token "masuk sebagai" (hanya-baca) milik Platform Admin
	platformadmin.NewHandler(platformadmin.HandlerDeps{
		Service: platformSvc, Log: d.Log, Redis: d.Redis,
		Lockout: auth.NewLockout(d.Redis, auth.NewPolicyLoader(d.Pool, d.Log)),
		Origins: d.Cfg.CORSOrigins, SecureCookie: !d.Cfg.IsDev(),
	}).Routes(r)

	iam.NewHandler(iam.NewService(d.Pool, perms, sessions), perms, tokens, d.Log).Routes(r)
	outlet.NewHandler(outlet.NewService(d.Pool, perms), perms, tokens, d.Log).Routes(r)
	audit.NewHandler(audit.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	catalog.NewHandler(catalog.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	item.NewHandler(item.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	return nil
}

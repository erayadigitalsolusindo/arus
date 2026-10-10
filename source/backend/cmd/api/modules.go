package main

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/accounting"
	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/dashboard"
	"aciraba/internal/iam"
	"aciraba/internal/item"
	"aciraba/internal/live"
	"aciraba/internal/member"
	"aciraba/internal/outlet"
	"aciraba/internal/payable"
	"aciraba/internal/paymentmethod"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/background"
	"aciraba/internal/platform/config"
	"aciraba/internal/platform/mailer"
	"aciraba/internal/platform/storage"
	"aciraba/internal/platformadmin"
	"aciraba/internal/posshortcut"
	"aciraba/internal/purchasing"
	"aciraba/internal/receivable"
	"aciraba/internal/sales"
	"aciraba/internal/shift"
	"aciraba/internal/stock"
	"aciraba/internal/voucher"
	"aciraba/internal/wallet"
)

// appDeps = infrastruktur bersama yang dirakit main() lalu diteruskan ke semua modul.
type appDeps struct {
	Cfg    config.Config
	Log    *slog.Logger
	Pool   *pgxpool.Pool
	Ctx    context.Context // umur proses: menghentikan langganan realtime saat shutdown
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

	liveHub := live.NewHub(d.Redis, d.Log)
	go liveHub.Run(d.Ctx)

	approvalSvc := approval.NewService(d.Pool, d.Redis, d.Cfg.JWTSecret)
	authSvc := auth.NewService(auth.Deps{
		Pool: d.Pool, Tokens: tokens, Sessions: sessions, OneTime: pauth.NewOneTime(d.Redis), Perms: perms,
		Mailer: d.Mailer, Jobs: d.Jobs, BaseURL: d.Cfg.AppBaseURL, Approvals: approvalSvc,
	})
	auth.NewHandler(auth.HandlerDeps{
		Service: authSvc, Log: d.Log, Redis: d.Redis,
		Lockout:    auth.NewLockout(d.Redis, auth.NewPolicyLoader(d.Pool, d.Log)),
		RateLimits: auth.NewRateLimitLoader(d.Pool, d.Log),
		Tokens:     tokens, Perms: perms, Origins: d.Cfg.CORSOrigins, SecureCookie: !d.Cfg.IsDev(),
	}).Routes(r)

	uploads, err := storage.NewLocal(d.Cfg.UploadDir)
	if err != nil {
		return err
	}

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
		APK: uploads,
	}).Routes(r)

	iam.NewHandler(iam.NewService(d.Pool, perms, sessions), perms, tokens, d.Log).Routes(r)
	outlet.NewHandler(outlet.NewService(d.Pool, perms), perms, tokens, d.Log).Routes(r)
	audit.NewHandler(audit.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	catalog.NewHandler(catalog.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	item.NewHandler(item.NewService(d.Pool, uploads), perms, tokens, d.Log).Routes(r)
	member.NewHandler(member.NewService(d.Pool, uploads), perms, tokens, d.Log).Routes(r)
	stock.NewHandler(stock.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	approval.NewHandler(approvalSvc, perms, tokens, d.Log).Routes(r)
	salesH := sales.NewHandler(sales.NewService(d.Pool, approvalSvc).WithShiftGuard(shift.Guard), perms, tokens, d.Log).WithNotifier(liveHub.Notify)
	live.NewHandler(live.NewService(d.Pool), liveHub, perms, tokens, d.Log).Routes(r)
	salesH.Routes(r)
	salesH.ReturnRoutes(r)
	receivableSvc := receivable.NewService(d.Pool)
	receivable.NewHandler(receivableSvc, perms, tokens, d.Log).Routes(r)
	posshortcut.NewHandler(posshortcut.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	voucher.NewHandler(voucher.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	paymentmethod.NewHandler(paymentmethod.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	purchaseH := purchasing.NewHandler(purchasing.NewService(d.Pool), perms, tokens, d.Log)
	purchaseH.Routes(r)
	purchaseH.ReturnRoutes(r)
	payableSvc := payable.NewService(d.Pool)
	payable.NewHandler(payableSvc, perms, tokens, d.Log).Routes(r)
	dashboard.NewHandler(dashboard.NewService(d.Pool, receivableSvc, payableSvc), perms, tokens, d.Log).Routes(r)
	wallet.NewHandler(wallet.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	shift.NewHandler(shift.NewService(d.Pool, approvalSvc), perms, tokens, d.Log).Routes(r)
	accounting.NewHandler(accounting.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	return nil
}

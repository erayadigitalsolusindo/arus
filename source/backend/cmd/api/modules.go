package main

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/accounting"
	"aciraba/internal/ai"
	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/dashboard"
	"aciraba/internal/erapay"
	"aciraba/internal/extapi"
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
	"aciraba/internal/plan"
	"aciraba/internal/platformadmin"
	"aciraba/internal/posshortcut"
	"aciraba/internal/purchasing"
	"aciraba/internal/receivable"
	reports_debt "aciraba/internal/reports/debt"
	reports_purchases "aciraba/internal/reports/purchases"
	reports_returns "aciraba/internal/reports/returns"
	reports_sales "aciraba/internal/reports/sales"
	reports_stock "aciraba/internal/reports/stock"
	"aciraba/internal/sales"
	"aciraba/internal/shift"
	"aciraba/internal/subscription"
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
	extStore, err := extapi.NewStore(d.Pool, d.Cfg.JWTSecret)
	if err != nil {
		return err
	}
	extCatalog := extapi.NewCatalogBuilder(r) // dibaca malas (saat dokumentasi dibuka), setelah semua rute terpasang
	platformSvc := platformadmin.NewService(platformadmin.Deps{
		Pool: d.Pool, Tokens: tokens, PTokens: pauth.NewPlatformTokenIssuer(d.Cfg.JWTSecret), Sessions: sessions,
		OneTime: pauth.NewOneTime(d.Redis), TOTP: totpBox, Perms: perms, SetupToken: d.Cfg.PlatformSetupToken,
		DomainTarget: d.Cfg.DomainTarget, Files: uploads,
	})
	go platformSvc.RunTenantPurger(d.Ctx, d.Log)
	perms.WithPlatform(platformSvc) // menerima token "masuk sebagai" (hanya-baca) milik Platform Admin
	platformadmin.NewHandler(platformadmin.HandlerDeps{
		Service: platformSvc, Log: d.Log, Redis: d.Redis,
		Lockout: auth.NewLockout(d.Redis, auth.NewPolicyLoader(d.Pool, d.Log)),
		Origins: d.Cfg.CORSOrigins, SecureCookie: !d.Cfg.IsDev(),
		APK: uploads, Ext: extStore, ExtCatalog: extCatalog,
	}).Routes(r)
	// API eksternal bertanda tangan HMAC (/ext/v1/*) meneruskan ke router yang sama; portal dokumentasi publik.
	extapi.NewGateway(extStore, d.Redis, tokens, r, d.Log).Mount(r)
	extapi.NewDocsHandler(extStore, extCatalog, d.Redis, d.Log).Routes(r)

	iam.NewHandler(iam.NewService(d.Pool, perms, sessions), perms, tokens, d.Log).Routes(r)
	outlet.NewHandler(outlet.NewService(d.Pool, perms), perms, tokens, d.Log).Routes(r)
	audit.NewHandler(audit.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	catalog.NewHandler(catalog.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	item.NewHandler(item.NewService(d.Pool, uploads), perms, tokens, d.Log).Routes(r)
	member.NewHandler(member.NewService(d.Pool, uploads), perms, tokens, d.Log).Routes(r)
	stock.NewHandler(stock.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	approval.NewHandler(approvalSvc, perms, tokens, d.Log).Routes(r)
	pmSvc := paymentmethod.NewService(d.Pool)
	eraSvc, err := erapay.NewService(d.Pool, d.Cfg.EraPayBaseURL, d.Cfg.JWTSecret, !d.Cfg.IsDev(), pmSvc)
	if err != nil {
		return err
	}
	eraSvc.WithProvisioningToken(d.Cfg.EraPayProvisionToken)
	salesSvc := sales.NewService(d.Pool, approvalSvc).WithShiftGuard(shift.Guard).WithPlanGuard(plan.ConsumeTx).WithPayVerifier(eraSvc)
	salesH := sales.NewHandler(salesSvc, perms, tokens, d.Log).WithNotifier(liveHub.Notify)
	live.NewHandler(live.NewService(d.Pool), liveHub, perms, tokens, d.Log).Routes(r)
	salesH.Routes(r)
	salesH.ReturnRoutes(r)
	receivableSvc := receivable.NewService(d.Pool)
	receivable.NewHandler(receivableSvc, perms, tokens, d.Log).Routes(r)
	posshortcut.NewHandler(posshortcut.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	voucher.NewHandler(voucher.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	paymentmethod.NewHandler(pmSvc, perms, tokens, d.Log).Routes(r)
	erapay.NewHandler(eraSvc, perms, tokens, d.Log).Routes(r)
	subSvc := subscription.NewService(d.Pool, d.Cfg.EraPayBaseURL, d.Cfg.EraPayPlatformKey)
	subscription.NewHandler(subSvc, perms, tokens, d.Log).Routes(r)
	purchaseH := purchasing.NewHandler(purchasing.NewService(d.Pool), perms, tokens, d.Log)
	purchaseH.Routes(r)
	purchaseH.ReturnRoutes(r)
	payableSvc := payable.NewService(d.Pool)
	payable.NewHandler(payableSvc, perms, tokens, d.Log).Routes(r)
	dashboard.NewHandler(dashboard.NewService(d.Pool, receivableSvc, payableSvc), perms, tokens, d.Log).Routes(r)
	wallet.NewHandler(wallet.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	shift.NewHandler(shift.NewService(d.Pool, approvalSvc), perms, tokens, d.Log).Routes(r)
	accSvc := accounting.NewService(d.Pool)
	accounting.NewHandler(accSvc, perms, tokens, d.Log).Routes(r)
	reportsSalesSvc := reports_sales.NewService(d.Pool)
	reports_sales.NewHandler(reportsSalesSvc, perms, tokens, d.Log).Routes(r)
	reports_stock.NewHandler(reports_stock.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	reports_purchases.NewHandler(reports_purchases.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	reports_returns.NewHandler(reports_returns.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	reports_debt.NewHandler(reports_debt.NewService(d.Pool), perms, tokens, d.Log).Routes(r)
	aiSvc, err := ai.NewService(d.Pool, d.Redis, d.Cfg.JWTSecret, ai.Free{Key: d.Cfg.AIPlatformKey, Limit: d.Cfg.AIFreeDailyLimit}, d.Log)
	if err != nil {
		return err
	}
	aiSvc.WithAPIBase(d.Cfg.APIBaseURL)
	ai.NewHandler(aiSvc, perms, tokens, d.Log).Routes(r)
	ai.NewMCPHandler(aiSvc, perms, tokens, d.Redis, d.Log, d.Cfg.APIBaseURL, d.Cfg.AppBaseURL).Routes(r)
	if d.Cfg.AutoJournal {
		go accounting.NewScheduler(accSvc, d.Pool, d.Log).Run(d.Ctx) // draf jurnal otomatis tiap malam; berhenti saat shutdown
	}
	if d.Cfg.EraPayReconcile {
		go subscription.NewReconciler(subSvc, d.Log).Run(d.Ctx) // aktivasi paket dari pembayaran QRIS yang lunas

		go sales.NewEraPayReconciler(salesSvc, d.Pool, d.Log).Run(d.Ctx) // verifikasi nota paksa ke EraPay; berhenti saat shutdown
	}
	return nil
}

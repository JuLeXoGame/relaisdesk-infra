package main

import (
	"context"
	dbpkg "database"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"api/config"
	"api/handlers"
	"api/mailer"
	"api/middleware"
	"api/networkauth"
)

// readSecretStdin reads a CLI secret (e.g. set-customer-password) from r,
// trimming the trailing newline a terminal or echo pipe adds.
func readSecretStdin(r io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	pass := strings.TrimSpace(string(raw))
	if pass == "" {
		return "", fmt.Errorf("mot de passe vide")
	}
	return pass, nil
}

func main() {
	cfg := config.LoadConfig()

	if len(os.Args) >= 3 && (os.Args[1] == "set-customer-password" || os.Args[1] == "set-password") {
		email := strings.ToLower(strings.TrimSpace(os.Args[2]))
		// The secret is never taken from argv (visible to all local users
		// via ps): it is read on stdin (terminal prompt or pipe).
		fmt.Fprint(os.Stderr, "Mot de passe: ")
		pass, err := readSecretStdin(os.Stdin)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			log.Fatalf("Lecture du mot de passe impossible: %v", err)
		}
		db, err := dbpkg.InitDatabase(cfg.DBPath)
		if err != nil {
			log.Fatalf("Initialisation base de données impossible: %v", err)
		}
		defer db.Close()
		if err := dbpkg.SetCustomerPassword(db, email, pass); err != nil {
			log.Fatalf("Erreur: %v", err)
		}
		log.Printf("Mot de passe défini avec succès pour %s", email)
		return
	}

	setupLogging(cfg.LogFile)
	if err := cfg.ValidateServerSettings(); err != nil {
		log.Fatalf("Invalid server configuration: %v", err)
	}
	if cfg.B2CSalesReady() && !cfg.ConsumerMediationConfigured() {
		log.Print("[LEGAL WARNING] B2C ouvert par décision explicite sans médiateur : non-conformité restant à régulariser ; aucune validation juridique implicite")
	}

	log.Println("=== RelaisDesk License API ===")
	log.Printf("Port: %s", cfg.APIPort)
	log.Printf("Database: %s", cfg.DBPath)

	if cfg.AdminToken == "CHANGE_ME_WITH_A_VERY_LONG_RANDOM_TOKEN" || len(cfg.AdminToken) < 32 {
		log.Fatal("ADMIN_TOKEN must be a unique random secret of at least 32 characters")
	}
	if (cfg.StripeSecretKey == "") != (cfg.StripeWebhookSecret == "") && !(cfg.DevHTTP && cfg.StripeMock) {
		log.Fatal("STRIPE_SECRET_KEY and STRIPE_WEBHOOK_SECRET must be configured together")
	}
	if cfg.StripeMock && !cfg.DevHTTP {
		log.Fatal("STRIPE_MOCK may only be enabled together with DEV_HTTP=true")
	}
	networkSigner, err := networkauth.LoadSigner(
		cfg.NetworkAuthPrivateKeyFile,
		cfg.NetworkAuthKeyID,
		time.Duration(cfg.NetworkTokenTTLSeconds)*time.Second,
	)
	if err != nil {
		log.Fatalf("Failed to initialize the network authorization signer: %v", err)
	}
	log.Printf("Network authorization key: %s (TTL %ds)", cfg.NetworkAuthKeyID, cfg.NetworkTokenTTLSeconds)

	db, err := dbpkg.InitDatabase(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	if cfg.StripeMode() != "" {
		if err := dbpkg.BindBillingEnvironment(db, cfg.StripeMode()); err != nil {
			log.Fatalf("Stripe : %v", err)
		}
	}
	settings := handlers.ServerSettings{
		ServerIP:       cfg.ServerIP,
		RendezvousPort: cfg.RendezvousPort,
		RelayPort:      cfg.RelayPort,
	}

	mail := mailer.NewMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	if !cfg.DevHTTP && !mail.IsConfigured() {
		log.Fatal("SMTP_HOST, SMTP_PORT and SMTP_FROM (plus SMTP_USER/SMTP_PASS when authentication is used) must be valid in production")
	}
	jobContext, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	go handlers.RunJobWorker(jobContext, db, cfg, mail)
	go handlers.RunOKXDepositWatcher(jobContext, db, cfg)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		runMaintenance := func() {
			dbpkg.PurgeExpiredSessions(db)
			cancelled, err := dbpkg.CancelExpiredOrders(db)
			if err == nil && cancelled > 0 {
				log.Printf("[Order Reaper] %d commandes expirées annulées", cancelled)
			}
			purged, err := dbpkg.PurgeExpiredOperationalData(db, time.Now().UTC())
			if err != nil {
				log.Printf("[Retention] échec de la purge: %v", err)
			} else if purged.ViewerCodes+purged.ConnectionLogs+purged.SecurityAlerts+purged.UnpaidOrders+purged.WithdrawalRecords+purged.Interventions+purged.CompletedJobs+purged.AuthBans > 0 {
				log.Printf("[Retention] purge: viewers=%d connexions=%d alertes=%d commandes=%d rétractations=%d interventions=%d travaux=%d bans=%d",
					purged.ViewerCodes, purged.ConnectionLogs, purged.SecurityAlerts, purged.UnpaidOrders, purged.WithdrawalRecords, purged.Interventions, purged.CompletedJobs, purged.AuthBans)
			}
			if carts, cartErr := handlers.QueueCartReminders(db, time.Now().UTC()); cartErr != nil {
				log.Printf("[Carts] relances indisponibles: %v", cartErr)
			} else if carts > 0 {
				log.Printf("[Carts] %d relance(s) panier mise(s) en file", carts)
			}
			reminders, reminderErr := handlers.ProcessRenewalReminders(db, cfg, mail, time.Now().UTC())
			if err := handlers.QueueTrialReminders(db, time.Now().UTC()); err != nil {
				log.Printf("[Trials] rappels indisponibles: %v", err)
			}
			if err := handlers.QueueTrialNurture(db, time.Now().UTC()); err != nil {
				log.Printf("[Trials] nurturing indisponible: %v", err)
			}
			if err := handlers.QueueSubscriptionRenewalNotices(db, time.Now().UTC()); err != nil {
				log.Printf("[Subscriptions] rappels annuels : %v", err)
			}
			if reminderErr != nil && mail.IsConfigured() {
				log.Printf("[Renewal Reminders] traitement incomplet: %v", reminderErr)
			} else if reminders > 0 {
				log.Printf("[Renewal Reminders] %d relance(s) mise(s) en file", reminders)
			}
		}
		runMaintenance()
		for range ticker.C {
			runMaintenance()
		}
	}()

	mux := http.NewServeMux()

	// Each sensitive credential endpoint has an independent limiter so traffic
	// to one feature cannot disable protection on another one.
	mux.Handle("/api/v1/activate", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.ActivateHandler(db, settings)))))
	mux.Handle("/api/v1/validate", middleware.StrictAuthLimiter(10, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.ValidateHandler(db)))))
	mux.Handle("/api/v1/heartbeat", middleware.RateLimit(120, time.Minute)(http.HandlerFunc(method(http.MethodPost, handlers.HeartbeatHandler(db)))))
	mux.HandleFunc("/api/v1/health", method(http.MethodGet, handlers.HealthHandler(db)))

	// Routes publiques (Site web relaisdesk.fr)
	mux.HandleFunc("/api/v1/public/pricing", method(http.MethodGet, handlers.PublicPricingHandler()))
	mux.HandleFunc("/api/v1/public/payment-methods", method(http.MethodGet, handlers.PublicPaymentMethodsHandler(cfg)))
	mux.HandleFunc("/api/v1/public/trials", method(http.MethodGet, handlers.TrialAvailabilityHandler(cfg)))
	mux.Handle("/api/v1/public/status", middleware.RateLimit(60, time.Minute)(http.HandlerFunc(method(http.MethodGet, handlers.PublicStatusHandler(db, settings, mail)))))
	mux.Handle("/api/v1/public/trials/request", middleware.RateLimit(5, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.PublicTrialRequestHandler(db, cfg)))))
	mux.Handle("/api/v1/public/trials/verify", middleware.StrictAuthLimiter(10, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.PublicTrialVerifyHandler(db, cfg)))))
	mux.HandleFunc("/api/v1/public/releases/latest", method(http.MethodGet, handlers.PublicReleaseManifestHandler(cfg)))
	mux.Handle("/api/v1/public/order", middleware.RateLimit(20, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.PublicOrderHandler(db, cfg, mail)))))
	mux.Handle("/api/v1/public/crypto/quote", middleware.RateLimit(20, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.CryptoQuoteRefreshHandler(db, cfg)))))
	mux.Handle("/api/v1/public/withdrawal", middleware.RateLimit(10, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.PublicWithdrawalHandler(db, mail)))))
	mux.Handle("/api/v1/public/trials/withdraw", middleware.RateLimit(10, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.PublicTrialWithdrawalHandler(db)))))
	mux.Handle("/api/v1/public/contact", middleware.RateLimit(10, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.PublicContactHandler(mail, cfg)))))
	mux.Handle("/api/v1/public/license/status", middleware.RateLimit(30, time.Minute)(http.HandlerFunc(method(http.MethodPost, handlers.PublicLicenseStatusHandler(db)))))
	mux.HandleFunc("/api/v1/downloads/", method(http.MethodGet, handlers.DownloadHandler(cfg)))
	mux.Handle("/api/v1/customer/login", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerLoginWithPasswordHandler(db)))))
	mux.Handle("/api/v1/customer/login/google", middleware.StrictAuthLimiter(10, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerGoogleLoginHandler(db, cfg.GoogleClientID)))))
	mux.Handle("/api/v1/customer/login/2fa", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerLogin2FAHandler(db)))))
	mux.Handle("/api/v1/customer/login/2fa/send-email-code", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FASendEmailCodeHandler(db, mail)))))
	mux.Handle("/api/v1/customer/login/request", middleware.StrictAuthLimiter(5, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerLoginRequestHandler(db, cfg, mail)))))
	mux.Handle("/api/v1/customer/login/verify", middleware.StrictAuthLimiter(10, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerLoginVerifyHandler(db)))))
	mux.Handle("/api/v1/customer/password/reset", middleware.StrictAuthLimiter(10, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerSetPasswordWithTokenHandler(db)))))
	mux.Handle("/api/v1/customer/2fa/setup-token", middleware.StrictAuthLimiter(10, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FASetupWithTokenHandler(db)))))
	mux.HandleFunc("/api/v1/customer/logout", method(http.MethodPost, handlers.CustomerLogoutHandler(db)))

	// Webhook Stripe
	mux.HandleFunc("/api/v1/stripe/webhook", method(http.MethodPost, handlers.StripeWebhookHandler(db, cfg, mail)))

	// Admin Login & Setup routes (strict limiter)
	mux.Handle("/api/v1/admin/login", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.AdminLoginHandler(db, cfg.AdminToken)))))
	mux.Handle("/api/v1/admin/password/setup", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.AdminPasswordSetupHandler(db)))))

	admin := func(next http.Handler) http.Handler {
		return middleware.AdminAuth(cfg.AdminToken, db)(next)
	}
	mux.Handle("/api/v1/admin/licences", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.AdminListLicensesHandler(db)(w, r)
		case http.MethodPost:
			handlers.AdminCreateLicenseHandler(db)(w, r)
		default:
			methodNotAllowed(w)
		}
	})))
	mux.Handle("/api/v1/admin/licences/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if strings.HasSuffix(r.URL.Path, "/revoke") {
				handlers.AdminRevokeHandler(db)(w, r)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/extend") {
				handlers.AdminExtendHandler(db)(w, r)
				return
			}
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/admin/stats", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminStatsHandler(db)))))
	mux.Handle("/api/v1/admin/system-status", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminSystemStatusHandler(db, settings, mail, networkSigner != nil)))))
	mux.Handle("/api/v1/admin/financials", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminFinancialsHandler(db)))))
	mux.Handle("/api/v1/admin/alerts", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminAlertsHandler(db)))))
	mux.Handle("/api/v1/admin/logout", admin(http.HandlerFunc(method(http.MethodPost, handlers.AdminLogoutHandler(db)))))

	mux.Handle("/api/v1/admin/orders", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminListOrdersHandler(db)))))
	mux.Handle("/api/v1/admin/withdrawals", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminListWithdrawalsHandler(db)))))
	mux.Handle("/api/v1/admin/orders/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handlers.AdminDeleteOrderHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/mark-paid") || strings.HasSuffix(r.URL.Path, "/retry-fulfillment")) {
			handlers.AdminMarkOrderPaidHandler(db, cfg, mail)(w, r)
			return
		}
		methodNotAllowed(w)
	})))

	// Invoices Routes
	mux.Handle("/api/v1/admin/invoices", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminListInvoicesHandler(db)))))
	mux.Handle("/api/v1/admin/invoices/upload", admin(http.HandlerFunc(method(http.MethodPost, handlers.AdminUploadInvoiceHandler(db, cfg)))))
	mux.Handle("/api/v1/admin/invoices/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handlers.AdminDeleteInvoiceHandler(db, cfg)(w, r)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/credit-note") {
			handlers.AdminCreateCreditNoteHandler(db, cfg)(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download") {
			handlers.AdminDownloadInvoiceHandler(db, cfg)(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cii") {
			handlers.AdminDownloadInvoiceCIIHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/send-email") {
			handlers.AdminSendInvoiceEmailHandler(db, cfg, mail)(w, r)
			return
		}
		methodNotAllowed(w)
	})))

	// Test Email Route (Phase A)
	mux.Handle("/api/v1/admin/test-email", admin(http.HandlerFunc(method(http.MethodPost, handlers.AdminTestEmailHandler(mail, cfg)))))

	mux.Handle("/api/v1/admin/viewer-codes/generate", admin(http.HandlerFunc(method(http.MethodPost, handlers.AdminGenerateViewerCodeHandler(db)))))
	mux.Handle("/api/v1/admin/viewer-codes", admin(http.HandlerFunc(method(http.MethodGet, handlers.AdminListViewerCodesHandler(db)))))
	mux.Handle("/api/v1/admin/viewer-codes/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/revoke") {
			handlers.AdminRevokeViewerCodeHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))

	mux.Handle("/api/v1/viewer/activate", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.ViewerActivateHandler(db, settings)))))
	mux.Handle("/api/v1/viewer/network-token", middleware.StrictAuthLimiter(30, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.ViewerNetworkTokenHandler(db, networkSigner)))))
	mux.Handle("/api/v1/viewer/announce", middleware.StrictAuthLimiter(10, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.ViewerAnnounceHandler(db)))))

	// Endpoints pour les postes distants permanents (Console de gestion / Unattended)
	// Enrollment codes carry 118+ bits of entropy with a 15-minute TTL
	// (park tokens 157 bits), so guessing is infeasible and the per-IP
	// budget can allow mass waves behind one NAT egress (120/min).
	mux.Handle("/api/v1/devices/enroll", middleware.StrictAuthLimiter(120, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.DeviceEnrollHandler(db, settings, networkSigner)))))
	// Heartbeats behind a corporate NAT share one egress IP: budget per
	// device (5/min is ~4x the 45s cadence) with a high per-IP flood guard.
	mux.Handle("/api/v1/devices/heartbeat", middleware.DeviceRateLimit(6000, 5, time.Minute)(http.HandlerFunc(method(http.MethodPost, handlers.DeviceHeartbeatHandler(db, cfg, networkSigner)))))

	// Customer commercial portal. Its e-mail session is intentionally separate
	// from technician licence credentials.
	customerAuth := middleware.CustomerAuth(db)
	mux.Handle("/api/v1/customer/service-billing", customerAuth(middleware.RateLimit(60, time.Minute)(http.HandlerFunc(handlers.CustomerServiceBillingHandler(db, cfg)))))
	mux.Handle("/api/v1/customer/service-billing/", customerAuth(middleware.RateLimit(60, time.Minute)(http.HandlerFunc(handlers.CustomerServiceBillingHandler(db, cfg)))))
	mux.HandleFunc("/api/v1/stripe/connect-webhook", method(http.MethodPost, handlers.ServiceStripeWebhookHandler(db, cfg)))
	mux.Handle("/api/v1/team/invitations/accept", middleware.StrictAuthLimiter(10, time.Hour, db)(http.HandlerFunc(method(http.MethodPost, handlers.TeamAcceptInvitationHandler(db)))))
	mux.Handle("/api/v1/customer/team", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.CustomerTeamHandler(db)))))
	mux.Handle("/api/v1/customer/team/invitations", customerAuth(middleware.RateLimit(30, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerTeamInviteHandler(db))))))
	mux.Handle("/api/v1/customer/team/members/", customerAuth(middleware.RateLimit(60, time.Hour)(http.HandlerFunc(handlers.CustomerTeamMemberHandler(db)))))
	mux.Handle("/api/v1/customer/team/technician-session", customerAuth(middleware.RateLimit(30, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerTeamTechnicianSessionHandler(db))))))
	mux.Handle("/api/v1/customer/subscriptions/cancel", customerAuth(middleware.RateLimit(20, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerCancelSubscriptionHandler(db, cfg))))))
	mux.Handle("/api/v1/customer/billing-portal", customerAuth(middleware.RateLimit(20, time.Hour)(http.HandlerFunc(method(http.MethodPost, handlers.CustomerBillingPortalHandler(db, cfg))))))
	mux.Handle("/api/v1/customer/dashboard", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.CustomerDashboardHandler(db)))))
	mux.Handle("/api/v1/customer/preferences", customerAuth(http.HandlerFunc(method(http.MethodPut, handlers.CustomerPreferencesHandler(db)))))
	mux.Handle("/api/v1/customer/preferences/password", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.CustomerChangePasswordHandler(db)))))
	mux.Handle("/api/v1/customer/licenses/", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/renew") {
			handlers.CustomerRenewLicenseHandler(db, cfg, mail)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/invoices/", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download") {
			handlers.CustomerDownloadInvoiceHandler(db, cfg)(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cii") {
			handlers.CustomerDownloadInvoiceCIIHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/interventions", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			handlers.CustomerInterventionsHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/interventions/export", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.CustomerInterventionsExportHandler(db)))))
	mux.Handle("/api/v1/customer/interventions/", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.CustomerInterventionActionHandler(db)))))
	mux.Handle("/api/v1/customer/viewer-codes/generate", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.CustomerGenerateViewerCodeHandler(db)))))
	mux.Handle("/api/v1/customer/viewer-codes", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.CustomerListViewerCodesHandler(db)))))
	mux.Handle("/api/v1/customer/viewer-codes/", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/revoke") {
			handlers.CustomerRevokeViewerCodeHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/devices", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.CustomerListDevicesHandler(db)))))
	mux.Handle("/api/v1/customer/devices/enrollment-code", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.CustomerCreateDeviceEnrollmentHandler(db)))))
	mux.Handle("/api/v1/customer/devices/", customerAuth(http.HandlerFunc(handlers.CustomerDeviceActionHandler(db))))
	mux.Handle("/api/v1/customer/device-park-tokens", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.CustomerListParkTokensHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handlers.CustomerCreateParkTokenHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/device-park-tokens/", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/revoke") {
			handlers.CustomerRevokeParkTokenHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/device-folders", customerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.CustomerListFoldersHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handlers.CustomerCreateFolderHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/customer/device-folders/", customerAuth(http.HandlerFunc(handlers.CustomerFolderActionHandler(db))))
	mux.Handle("/api/v1/customer/2fa/setup", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FASetupHandler(db)))))
	mux.Handle("/api/v1/customer/2fa/enable", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FAEnableHandler(db)))))
	mux.Handle("/api/v1/customer/2fa/disable", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FADisableHandler(db)))))
	mux.Handle("/api/v1/customer/2fa/status", customerAuth(http.HandlerFunc(method(http.MethodGet, handlers.Customer2FAStatusHandler(db)))))
	mux.Handle("/api/v1/customer/2fa/recovery-codes", customerAuth(http.HandlerFunc(method(http.MethodPost, handlers.Customer2FARecoveryCodesHandler(db)))))

	// Technician Routes
	mux.Handle("/api/v1/technician/login", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianLoginHandler(db, settings)))))
	mux.Handle("/api/v1/technician/login/google", middleware.StrictAuthLimiter(10, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianGoogleLoginHandler(db, settings, cfg.GoogleClientID)))))
	mux.Handle("/api/v1/technician/login/2fa", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianLogin2FAHandler(db, settings)))))
	mux.Handle("/api/v1/technician/login/email-code", middleware.StrictAuthLimiter(5, time.Minute, db)(http.HandlerFunc(method(http.MethodPost, handlers.Technician2FASendEmailCodeHandler(db, mail)))))
	mux.HandleFunc("/api/v1/technician/logout", method(http.MethodPost, handlers.TechnicianLogoutHandler(db)))

	techAuth := middleware.TechnicianAuth(db)
	mux.Handle("/api/v1/technician/service-billing", techAuth(middleware.RateLimit(120, time.Minute)(http.HandlerFunc(handlers.TechnicianServiceBillingHandler(db, cfg)))))
	mux.Handle("/api/v1/technician/service-billing/", techAuth(middleware.ServiceOperationRateLimit(http.HandlerFunc(handlers.TechnicianServiceBillingHandler(db, cfg)))))
	mux.Handle("/api/v1/technician/network-token", techAuth(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianNetworkTokenHandler(db, networkSigner)))))
	mux.Handle("/api/v1/technician/dashboard", techAuth(http.HandlerFunc(method(http.MethodGet, handlers.TechnicianDashboardHandler(db)))))
	mux.Handle("/api/v1/technician/viewer-codes/generate", techAuth(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianGenerateCodeHandler(db)))))
	mux.Handle("/api/v1/technician/devices", techAuth(http.HandlerFunc(method(http.MethodGet, handlers.TechnicianListDevicesHandler(db)))))
	mux.Handle("/api/v1/technician/devices/enrollment-code", techAuth(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianCreateDeviceEnrollmentHandler(db)))))
	mux.Handle("/api/v1/technician/devices/", techAuth(http.HandlerFunc(handlers.TechnicianDeviceActionHandler(db))))
	mux.Handle("/api/v1/technician/device-park-tokens", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.TechnicianListParkTokensHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handlers.TechnicianCreateParkTokenHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/technician/device-park-tokens/", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/revoke") {
			handlers.TechnicianRevokeParkTokenHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/technician/device-folders", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.TechnicianListFoldersHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handlers.TechnicianCreateFolderHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/technician/device-folders/", techAuth(http.HandlerFunc(handlers.TechnicianFolderActionHandler(db))))
	mux.Handle("/api/v1/technician/interventions", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			handlers.TechnicianInterventionsHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))
	mux.Handle("/api/v1/technician/interventions/", techAuth(http.HandlerFunc(method(http.MethodPost, handlers.TechnicianInterventionActionHandler(db)))))
	mux.Handle("/api/v1/technician/viewer-codes/", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.HasSuffix(r.URL.Path, "/connect") {
			handlers.TechnicianConnectCodeHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/revoke") {
			handlers.TechnicianRevokeCodeHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodDelete || (r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/delete")) {
			handlers.TechnicianDeleteCodeHandler(db)(w, r)
			return
		}
		methodNotAllowed(w)
	})))

	handler := middleware.SecurityHeaders(
		middleware.Recovery(log.Default())(
			middleware.Logging(log.Default())(
				middleware.CORS(
					middleware.ServiceAwareRateLimit(db, 300, time.Minute)(
						middleware.LimitRequestBodyWithOverrides(1<<20, map[string]int64{
							"/api/v1/admin/invoices/upload": 11 << 20,
						})(mux),
					),
				),
			),
		),
	)

	addr := net.JoinHostPort(cfg.APIBind, cfg.APIPort)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			log.Printf("Starting HTTPS server on %s", addr)
			if err := srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server failed: %v", err)
			}
		} else if cfg.DevHTTP {
			log.Println("WARNING: DEV_HTTP=true set. Starting HTTP server (NOT FOR PRODUCTION).")
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server failed: %v", err)
			}
		} else {
			log.Fatalf("TLS_CERT and TLS_KEY are required in production. Set DEV_HTTP=true to run in HTTP mode for testing.")
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server gracefully...")
	stopJobs()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown failed: %v", err)
	}
}

func method(expected string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != expected {
			methodNotAllowed(w)
			return
		}
		next(w, r)
	}
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	w.Write([]byte(`{"error":"method not allowed"}`))
}

func setupLogging(logFile string) {
	if logFile == "" {
		return
	}

	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		log.Printf("WARNING: Failed to create log directory %s: %v", logDir, err)
		return
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("WARNING: Failed to open log file %s: %v", logFile, err)
		return
	}

	log.SetOutput(io.MultiWriter(os.Stdout, f))
}

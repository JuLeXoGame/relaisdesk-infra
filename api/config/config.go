package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIBind                   string
	APIPort                   string
	DBPath                    string
	AdminToken                string
	ServerIP                  string
	RendezvousPort            int
	RelayPort                 int
	TLSCert                   string
	TLSKey                    string
	LogFile                   string
	DevHTTP                   bool
	NetworkAuthPrivateKeyFile string
	NetworkAuthKeyID          string
	NetworkTokenTTLSeconds    int

	// Stripe configuration
	StripeSecretKey            string
	StripeWebhookSecret        string
	ServicePaymentsEnabled     bool
	StripeConnectWebhookSecret string
	StripeMock                 bool
	StripeSuccessURL           string
	StripeCancelURL            string
	TrialsEnabled              bool
	TrialFingerprintKey        string
	TOTPDataKey                string

	// SMTP configuration (Strictly port 587 STARTTLS or 465 TLS direct)
	SMTPHost            string
	SMTPPort            int
	SMTPUser            string
	SMTPPass            string
	SMTPFrom            string
	EmailDomain         string
	SMTPDKIMSelector    string
	EmailRequireDNSAuth bool

	// Bank transfer details
	BankIBAN   string
	BankBIC    string
	BankHolder string

	// Crypto payments collected on the operator OKX account (read-only API
	// key): quotes price at the OKX spot rate, deposits are detected by
	// polling the OKX deposit history. Nothing is offered until CryptoEnabled
	// is set AND the asset deposit configuration is complete.
	CryptoEnabled         bool
	CryptoQuoteTTLMinutes int
	OKXBaseURL            string
	OKXAPIKey             string
	OKXAPISecret          string
	OKXAPIPassphrase      string
	OKXBTCDepositAddress  string
	OKXXRPDepositAddress  string
	OKXXRPDepositTag      string

	// Downloads & Web configuration
	DownloadsDir        string
	InvoicesDir         string
	PublicWebsiteURL    string
	ClientSourceURL     string
	ServerSourceURL     string
	ReleaseManifestPath string
	ReleasePublicKey    string
	ReleaseKeyID        string
	MacBetaToken        string

	// Explicit operating decision; an override is not a declaration of compliance.
	B2CSalesEnabled                 bool
	B2CMediationPendingAcknowledged bool
	LegalPhone                      string
	ConsumerMediatorName            string
	ConsumerMediatorURL             string
	GoogleClientID                  string
}

const placeholderBankIBAN = "FR7600000000000000000000000"

func init() {
	loadDotEnv()
}

// loadDotEnv searches and loads environment variables from .env files if present.
func loadDotEnv() {
	envFiles := []string{
		".env",
		"api/.env",
		"../.env",
		"/etc/relaisdesk/.env",
		"/etc/relaisdesk/api.env",
		"/data/relaisdesk/.env",
	}

	for _, path := range envFiles {
		if file, err := os.Open(path); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					// Trim surrounding quotes
					val = strings.Trim(val, `"'`)
					// Do not overwrite existing environment variables
					if _, exists := os.LookupEnv(key); !exists {
						_ = os.Setenv(key, val)
					}
				}
			}
			_ = file.Close()
			log.Printf("[Config] Loaded environment variables from %s", path)
			break
		}
	}
}

func LoadConfig() *Config {
	invoicesDir := getEnv("INVOICES_DIR", "/data/relaisdesk/invoices")
	// Ensure directory exists with secure 0700 permissions
	if err := os.MkdirAll(invoicesDir, 0700); err != nil {
		// Fallback to local data/invoices in development
		invoicesDir = filepath.Join("data", "invoices")
		_ = os.MkdirAll(invoicesDir, 0700)
	}

	return &Config{
		APIBind:                         getEnv("API_BIND", "127.0.0.1"),
		APIPort:                         getEnv("API_PORT", "8443"),
		DBPath:                          getEnv("DB_PATH", "/data/relaisdesk/licences.db"),
		AdminToken:                      getEnv("ADMIN_TOKEN", "CHANGE_ME_WITH_A_VERY_LONG_RANDOM_TOKEN"),
		ServerIP:                        getEnv("SERVER_IP", "api.relaisdesk.fr"),
		RendezvousPort:                  getEnvInt("RUSTDESK_RENDEZVOUS_PORT", 21116),
		RelayPort:                       getEnvInt("RUSTDESK_RELAY_PORT", 21117),
		TLSCert:                         getEnv("TLS_CERT", ""),
		TLSKey:                          getEnv("TLS_KEY", ""),
		LogFile:                         getEnv("LOG_FILE", "/var/log/relaisdesk/api.log"),
		DevHTTP:                         getEnv("DEV_HTTP", "false") == "true",
		NetworkAuthPrivateKeyFile:       getEnv("NETWORK_AUTH_PRIVATE_KEY_FILE", "/etc/relaisdesk/network-auth-ed25519"),
		NetworkAuthKeyID:                getEnv("NETWORK_AUTH_KEY_ID", "relaisdesk-1"),
		NetworkTokenTTLSeconds:          getEnvInt("NETWORK_TOKEN_TTL_SECONDS", 300),
		StripeSecretKey:                 getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:             getEnv("STRIPE_WEBHOOK_SECRET", ""),
		ServicePaymentsEnabled:          getEnv("SERVICE_PAYMENTS_ENABLED", "false") == "true",
		StripeConnectWebhookSecret:      getEnv("STRIPE_CONNECT_WEBHOOK_SECRET", ""),
		StripeMock:                      getEnv("STRIPE_MOCK", "false") == "true",
		StripeSuccessURL:                getEnv("STRIPE_SUCCESS_URL", "https://relaisdesk.fr/?order=success"),
		StripeCancelURL:                 getEnv("STRIPE_CANCEL_URL", "https://relaisdesk.fr/?order=cancel"),
		TrialsEnabled:                   getEnv("TRIALS_ENABLED", "false") == "true",
		TrialFingerprintKey:             getEnv("TRIAL_FINGERPRINT_KEY", ""),
		TOTPDataKey:                     getEnv("TOTP_DATA_KEY", ""),
		SMTPHost:                        getEnv("SMTP_HOST", ""),
		SMTPPort:                        getEnvInt("SMTP_PORT", 587),
		SMTPUser:                        getEnv("SMTP_USER", ""),
		SMTPPass:                        getEnv("SMTP_PASS", ""),
		SMTPFrom:                        getEnv("SMTP_FROM", "RelaisDesk <contact@relaisdesk.fr>"),
		EmailDomain:                     strings.ToLower(getEnv("EMAIL_DOMAIN", "relaisdesk.fr")),
		SMTPDKIMSelector:                getEnv("SMTP_DKIM_SELECTOR", ""),
		EmailRequireDNSAuth:             getEnv("EMAIL_REQUIRE_DNS_AUTH", "true") == "true",
		BankIBAN:                        getEnv("BANK_IBAN", "FR76 0000 0000 0000 0000 0000 000"),
		BankBIC:                         getEnv("BANK_BIC", "BNPAFRPP"),
		BankHolder:                      getEnv("BANK_HOLDER", "Informatique A Domicile 03 / RelaisDesk"),
		CryptoEnabled:                   getEnv("CRYPTO_ENABLED", "false") == "true",
		CryptoQuoteTTLMinutes:           getEnvInt("CRYPTO_QUOTE_TTL_MINUTES", 30),
		OKXBaseURL:                      strings.TrimRight(getEnv("OKX_BASE_URL", "https://www.okx.com"), "/"),
		OKXAPIKey:                       strings.TrimSpace(getEnv("OKX_API_KEY", "")),
		OKXAPISecret:                    strings.TrimSpace(getEnv("OKX_API_SECRET", "")),
		OKXAPIPassphrase:                strings.TrimSpace(getEnv("OKX_API_PASSPHRASE", "")),
		OKXBTCDepositAddress:            strings.TrimSpace(getEnv("OKX_BTC_DEPOSIT_ADDRESS", "")),
		OKXXRPDepositAddress:            strings.TrimSpace(getEnv("OKX_XRP_DEPOSIT_ADDRESS", "")),
		OKXXRPDepositTag:                strings.TrimSpace(getEnv("OKX_XRP_DEPOSIT_TAG", "")),
		DownloadsDir:                    getEnv("DOWNLOADS_DIR", "installer/build"),
		InvoicesDir:                     invoicesDir,
		PublicWebsiteURL:                getEnv("PUBLIC_WEBSITE_URL", "https://relaisdesk.fr"),
		ClientSourceURL:                 getEnv("CLIENT_SOURCE_URL", ""),
		ServerSourceURL:                 getEnv("SERVER_SOURCE_URL", ""),
		ReleaseManifestPath:             getEnv("RELEASE_MANIFEST_PATH", "/opt/relaisdesk/downloads/release-manifest.json"),
		ReleasePublicKey:                getEnv("RELEASE_PUBLIC_KEY", ""),
		ReleaseKeyID:                    getEnv("RELEASE_KEY_ID", "release-1"),
		MacBetaToken:                    getEnv("MAC_BETA_TOKEN", ""),
		B2CSalesEnabled:                 getEnv("B2C_SALES_ENABLED", "false") == "true",
		B2CMediationPendingAcknowledged: getEnv("B2C_MEDIATION_PENDING_ACKNOWLEDGED", "false") == "true",
		LegalPhone:                      getEnv("LEGAL_PHONE", ""),
		ConsumerMediatorName:            getEnv("CONSUMER_MEDIATOR_NAME", ""),
		ConsumerMediatorURL:             getEnv("CONSUMER_MEDIATOR_URL", ""),
		GoogleClientID:                  getEnv("GOOGLE_CLIENT_ID", "226991768980-c2rcfkicoft0hl9m346n45ad088utri9.apps.googleusercontent.com"),
	}
}

// BankTransferConfigured rejects placeholders and validates the IBAN checksum
// before bank details are ever returned to a customer.
func (c *Config) BankTransferConfigured() bool {
	iban := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(c.BankIBAN), " ", ""))
	if iban == "" || iban == placeholderBankIBAN || len(iban) < 15 || len(iban) > 34 {
		return false
	}
	for i, char := range iban {
		isLetter := char >= 'A' && char <= 'Z'
		isDigit := char >= '0' && char <= '9'
		if (!isLetter && !isDigit) || (i < 2 && !isLetter) || (i >= 2 && i < 4 && !isDigit) {
			return false
		}
	}

	remainder := 0
	for _, char := range iban[4:] + iban[:4] {
		if char >= '0' && char <= '9' {
			remainder = (remainder*10 + int(char-'0')) % 97
		} else {
			value := int(char-'A') + 10
			remainder = (remainder*100 + value) % 97
		}
	}
	if remainder != 1 {
		return false
	}

	bic := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(c.BankBIC), " ", ""))
	if len(bic) != 8 && len(bic) != 11 {
		return false
	}
	for i, char := range bic {
		if i < 6 {
			if char < 'A' || char > 'Z' {
				return false
			}
		} else if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return strings.TrimSpace(c.BankHolder) != ""
}

// CryptoQuoteTTL returns the quote validity clamped to 5..30 minutes.
func (c *Config) CryptoQuoteTTL() time.Duration {
	ttl := c.CryptoQuoteTTLMinutes
	if ttl < 5 {
		ttl = 5
	}
	if ttl > 30 {
		ttl = 30
	}
	return time.Duration(ttl) * time.Minute
}

// CryptoAssetConfigured reports whether an asset ("BTC" or "XRP") can be
// offered: globally enabled, OKX reachable in principle, the read-only API
// credentials set, and the asset deposit address (plus tag for XRP)
// configured.
func (c *Config) CryptoAssetConfigured(asset string) bool {
	if c == nil || !c.CryptoEnabled || strings.TrimSpace(c.OKXBaseURL) == "" {
		return false
	}
	if strings.TrimSpace(c.OKXAPIKey) == "" || strings.TrimSpace(c.OKXAPISecret) == "" ||
		strings.TrimSpace(c.OKXAPIPassphrase) == "" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(asset)) {
	case "BTC":
		return strings.TrimSpace(c.OKXBTCDepositAddress) != ""
	case "XRP":
		if strings.TrimSpace(c.OKXXRPDepositAddress) == "" {
			return false
		}
		_, err := c.OKXXRPDepositTagValue()
		return err == nil
	default:
		return false
	}
}

// OKXXRPDepositTagValue parses the fixed OKX XRP deposit tag/memo shown on
// every XRP quote. The tag is identical for all orders: deposits are told
// apart by amount and time window.
func (c *Config) OKXXRPDepositTagValue() (uint32, error) {
	raw := strings.TrimSpace(c.OKXXRPDepositTag)
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || raw == "" {
		return 0, fmt.Errorf("tag de dépôt XRP invalide")
	}
	return uint32(value), nil
}

// ValidateServerSettings prevents a malformed environment variable from being
// propagated into every generated RustDesk configuration.
func (c *Config) ValidateServerSettings() error {
	if c.ServicePaymentsEnabled && (c.StripeMode() == "" || c.StripeConnectWebhookSecret == "" || c.StripeMock || !validPublicHTTPSURL(c.PublicWebsiteURL)) {
		return fmt.Errorf("les prestations exigent Stripe Connect, un secret de webhook connecté et PUBLIC_WEBSITE_URL HTTPS")
	}
	if c.TrialsEnabled && !c.TrialsReady() {
		return fmt.Errorf("les essais exigent Stripe, SMTP et TRIAL_FINGERPRINT_KEY (32 octets aléatoires en base64url)")
	}
	apiBind := strings.TrimSpace(c.APIBind)
	if apiBind == "" {
		apiBind = "127.0.0.1"
	}
	bindIP := net.ParseIP(apiBind)
	if bindIP == nil {
		return fmt.Errorf("API_BIND doit être une adresse IP valide")
	}
	if !c.DevHTTP && !bindIP.IsLoopback() {
		return fmt.Errorf("API_BIND doit être une adresse de bouclage en production")
	}
	apiPort, err := strconv.Atoi(strings.TrimSpace(c.APIPort))
	if err != nil || apiPort < 1 || apiPort > 65535 {
		return fmt.Errorf("API_PORT doit être un port valide (1-65535)")
	}
	if !validServerHost(c.ServerIP) {
		return fmt.Errorf("SERVER_IP doit être une adresse IP ou un nom DNS valide, sans protocole ni port")
	}
	if c.RendezvousPort < 1 || c.RendezvousPort > 65535 {
		return fmt.Errorf("RUSTDESK_RENDEZVOUS_PORT doit être compris entre 1 et 65535")
	}
	if c.RelayPort < 1 || c.RelayPort > 65535 {
		return fmt.Errorf("RUSTDESK_RELAY_PORT doit être compris entre 1 et 65535")
	}
	if strings.TrimSpace(c.NetworkAuthPrivateKeyFile) == "" {
		return fmt.Errorf("NETWORK_AUTH_PRIVATE_KEY_FILE est obligatoire")
	}
	if !validKeyID(c.NetworkAuthKeyID) {
		return fmt.Errorf("NETWORK_AUTH_KEY_ID est invalide")
	}
	if c.NetworkTokenTTLSeconds < 60 || c.NetworkTokenTTLSeconds > 900 {
		return fmt.Errorf("NETWORK_TOKEN_TTL_SECONDS doit être compris entre 60 et 900")
	}
	if !c.DevHTTP {
		if !validPublicHTTPSURL(c.PublicWebsiteURL) {
			return fmt.Errorf("PUBLIC_WEBSITE_URL doit être une URL HTTPS publique valide")
		}
		if !validPublicHTTPSURL(c.StripeSuccessURL) || !validPublicHTTPSURL(c.StripeCancelURL) {
			return fmt.Errorf("STRIPE_SUCCESS_URL et STRIPE_CANCEL_URL doivent être des URL HTTPS valides")
		}
		if !sameURLHostname(c.PublicWebsiteURL, c.StripeSuccessURL) || !sameURLHostname(c.PublicWebsiteURL, c.StripeCancelURL) {
			return fmt.Errorf("les URL de retour Stripe doivent utiliser le même nom d'hôte que PUBLIC_WEBSITE_URL")
		}
	}
	if !c.DevHTTP && !c.SourceDistributionReady() {
		return fmt.Errorf("CLIENT_SOURCE_URL et SERVER_SOURCE_URL doivent être des URL HTTPS publiques avant toute mise en production AGPLv3")
	}
	if !c.DevHTTP && (strings.TrimSpace(c.ReleaseManifestPath) == "" || !validEd25519PublicKey(c.ReleasePublicKey)) {
		return fmt.Errorf("RELEASE_MANIFEST_PATH et RELEASE_PUBLIC_KEY Ed25519 sont obligatoires en production")
	}
	if strings.TrimSpace(c.ReleaseKeyID) == "" {
		return fmt.Errorf("RELEASE_KEY_ID ne doit pas être vide")
	}
	if !c.DevHTTP && !validLegalPhone(c.LegalPhone) {
		return fmt.Errorf("LEGAL_PHONE doit contenir le numéro professionnel publié avant toute mise en production")
	}
	if c.B2CSalesEnabled && !c.B2CSalesReady() {
		return fmt.Errorf("ventes B2C : téléphone requis et médiateur à configurer (ou décision explicite B2C_MEDIATION_PENDING_ACKNOWLEDGED, qui ne rend pas l'ouverture conforme)")
	}
	if !c.DevHTTP && c.EmailRequireDNSAuth && strings.TrimSpace(c.SMTPHost) != "" {
		parsedFrom, err := mail.ParseAddress(strings.TrimSpace(c.SMTPFrom))
		if err != nil || !strings.EqualFold(emailDomain(parsedFrom.Address), strings.TrimSpace(c.EmailDomain)) {
			return fmt.Errorf("SMTP_FROM doit appartenir à EMAIL_DOMAIN lorsque EMAIL_REQUIRE_DNS_AUTH=true")
		}
		if !validDNSLabel(c.SMTPDKIMSelector) {
			return fmt.Errorf("SMTP_DKIM_SELECTOR doit être le sélecteur DKIM publié par votre fournisseur")
		}
	}
	return nil
}

func (c *Config) TrialKey() []byte {
	key, err := base64.RawURLEncoding.DecodeString(c.TrialFingerprintKey)
	if err != nil || len(key) != 32 {
		return nil
	}
	return key
}

// TOTPKey decodes the at-rest encryption key for TOTP secrets (32 random
// bytes, base64url). Nil when absent or malformed: TOTP writes fail closed.
func (c *Config) TOTPKey() []byte {
	key, err := base64.RawURLEncoding.DecodeString(c.TOTPDataKey)
	if err != nil || len(key) != 32 {
		return nil
	}
	return key
}

func (c *Config) StripeMode() string {
	if strings.HasPrefix(c.StripeSecretKey, "sk_test_") || strings.HasPrefix(c.StripeSecretKey, "rk_test_") {
		return "test"
	}
	if strings.HasPrefix(c.StripeSecretKey, "sk_live_") || strings.HasPrefix(c.StripeSecretKey, "rk_live_") {
		return "live"
	}
	return ""
}

func (c *Config) TrialsReady() bool {
	from, err := mail.ParseAddress(c.SMTPFrom)
	return c.TrialsEnabled && len(c.TrialKey()) == 32 && c.StripeSecretKey != "" && c.StripeWebhookSecret != "" && c.SMTPHost != "" &&
		!strings.ContainsAny(c.SMTPHost, "\r\n\t /:") && (c.SMTPPort == 465 || c.SMTPPort == 587) && err == nil && from.Address != "" && ((c.SMTPUser == "") == (c.SMTPPass == ""))
}

func emailDomain(address string) string {
	parts := strings.Split(address, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

func validDNSLabel(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

// SourceDistributionReady prevents production distribution before corresponding sources are public.
func (c *Config) SourceDistributionReady() bool {
	return validPublicHTTPSURL(c.ClientSourceURL) && validPublicHTTPSURL(c.ServerSourceURL)
}

func validEd25519PublicKey(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == 32
}

// B2CSalesReady is operational readiness only, not legal approval. The default
// still blocks absent mediation; a deliberate operator override is auditable.
func (c *Config) B2CSalesReady() bool {
	return c != nil && c.B2CSalesEnabled && validLegalPhone(c.LegalPhone) &&
		(c.ConsumerMediationConfigured() || c.B2CMediationPendingAcknowledged)
}

func (c *Config) ConsumerMediationConfigured() bool {
	return c != nil && strings.TrimSpace(c.ConsumerMediatorName) != "" && validPublicHTTPSURL(c.ConsumerMediatorURL)
}

func validLegalPhone(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 10 || len(value) > 32 {
		return false
	}
	digits := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits++
			continue
		}
		if !strings.ContainsRune("+(). -", char) {
			return false
		}
	}
	return digits >= 10
}

func validPublicHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func sameURLHostname(first, second string) bool {
	firstURL, firstErr := url.Parse(strings.TrimSpace(first))
	secondURL, secondErr := url.Parse(strings.TrimSpace(second))
	return firstErr == nil && secondErr == nil && firstURL.Hostname() != "" &&
		strings.EqualFold(firstURL.Hostname(), secondURL.Hostname())
}

func validKeyID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func validServerHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

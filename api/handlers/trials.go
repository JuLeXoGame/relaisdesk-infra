package handlers

import (
	"api/config"
	"api/mailer"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const jobTrialVerify = "trial_verify_email"
const jobTrialNotice = "trial_notice_email"
const jobTrialCancel = "trial_cancel"

type trialNotice struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// LicenseKey carries the plaintext trial key for one-time delivery on
	// "activated" notices. The worker scrubs it from the stored payload
	// after a successful send; other kinds leave it empty.
	LicenseKey string `json:"license_key,omitempty"`
}
type trialRequest struct {
	PublicOrderRequest
	RecurringAccepted  bool   `json:"recurring_accepted"`
	TrialTermsVersion  string `json:"trial_terms_version"`
	ExpectedPriceCents int64  `json:"expected_price_cents"`
}

func TrialAvailabilityHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": cfg.TrialsReady(), "days": dbpkg.TrialDays, "terms_version": publicTermsVersion, "trial_terms_version": dbpkg.TrialTermsVersion, "consumer_enabled": cfg.B2CSalesReady(), "mediation_pending": cfg.B2CSalesReady() && !cfg.ConsumerMediationConfigured()})
	}
}

func PublicTrialRequestHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.TrialsReady() {
			writeJSONError(w, "Les essais ne sont pas encore ouverts", 503)
			return
		}
		var req trialRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", 400)
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		parsed, err := mail.ParseAddress(req.Email)
		if err != nil || parsed.Address != req.Email || len(req.Email) > 254 || strings.ContainsAny(req.Email, "\r\n\t") {
			writeJSONError(w, "Adresse e-mail invalide", 400)
			return
		}
		if !req.TermsAccepted || req.TermsVersion != publicTermsVersion || !req.RecurringAccepted || req.TrialTermsVersion != dbpkg.TrialTermsVersion {
			writeJSONError(w, "Acceptez les CGV et le prélèvement récurrent après les 30 jours gratuits", 400)
			return
		}
		if req.CustomerType != "business" && req.CustomerType != "consumer" {
			writeJSONError(w, "Statut client invalide", 400)
			return
		}
		if req.CustomerType == "consumer" && !cfg.B2CSalesReady() {
			writeJSONError(w, "Les ventes aux consommateurs ne sont pas encore ouvertes", 503)
			return
		}
		if req.CustomerType == "consumer" && !req.ImmediatePerformanceRequested {
			writeJSONError(w, "Demande expresse d'activation immédiate requise", 400)
			return
		}
		b := dbpkg.BillingDetails{Name: strings.TrimSpace(req.Name), Address: strings.TrimSpace(req.Address), PostalCode: strings.TrimSpace(req.PostalCode), City: strings.TrimSpace(req.City), Country: strings.TrimSpace(req.Country), SIRET: strings.ReplaceAll(strings.TrimSpace(req.SIRET), " ", ""), CustomerType: req.CustomerType, TermsVersion: dbpkg.TrialTermsVersion, TermsAccepted: true, ImmediatePerformanceRequested: req.ImmediatePerformanceRequested, BillingCycle: req.BillingCycle}
		if b.Name == "" || b.Address == "" || b.PostalCode == "" || b.City == "" || len(b.Name) > 200 || len(b.Address) > 500 || len(b.PostalCode) > 32 || len(b.City) > 100 || len(b.Country) > 100 || len(b.SIRET) > 32 {
			writeJSONError(w, "Nom et adresse de facturation complets requis", 400)
			return
		}
		if b.Country == "" {
			b.Country = "France"
		}
		if b.SIRET != "" {
			if len(b.SIRET) != 14 {
				writeJSONError(w, "SIRET : 14 chiffres attendus", 400)
				return
			}
			for _, c := range b.SIRET {
				if c < '0' || c > '9' {
					writeJSONError(w, "SIRET invalide", 400)
					return
				}
			}
		}
		if b.BillingCycle != "monthly" && b.BillingCycle != "annual" {
			writeJSONError(w, "Périodicité invalide", 400)
			return
		}
		price, _, _, err := dbpkg.CalculateServerPrice(req.Plan, req.Technicians, b.BillingCycle)
		if err != nil {
			writeJSONError(w, "Offre invalide", 400)
			return
		}
		if req.ExpectedPriceCents != int64(price*100+0.5) {
			writeJSONError(w, "Le tarif a changé. Actualisez et vérifiez le montant avant de confirmer.", 409)
			return
		}
		t, err := dbpkg.CreateTrialApplication(db, req.Email, req.Plan, req.Technicians, b, time.Now().UTC())
		if err == nil {
			_, err = enqueueJSONJob(db, jobTrialVerify, stringPayload{Value: t.ID}, "trial-verify:"+t.ID, 8)
		}
		if err != nil && !errors.Is(err, dbpkg.ErrTrialUnavailable) {
			writeJSONError(w, "Impossible de préparer la demande", 500)
			return
		}
		writeJSON(w, 202, map[string]string{"message": "Si votre demande peut être traitée, un lien de confirmation vous sera envoyé. Vérifiez aussi les courriers indésirables."})
	}
}

func PublicTrialVerifyHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.TrialsReady() {
			writeJSONError(w, "Les essais sont indisponibles", 503)
			return
		}
		var req customerVerifyRequest
		if err := decodeSingleJSON(r, &req); err != nil || len(req.Token) < 32 || len(req.Token) > 256 {
			writeJSONError(w, "Lien invalide ou expiré", 400)
			return
		}
		t, err := dbpkg.VerifyTrialEmail(db, req.Token, time.Now().UTC())
		if err != nil {
			writeJSONError(w, "Lien invalide ou expiré", 400)
			return
		}
		checkout, err := trialCheckout(cfg, db, t)
		if err != nil {
			writeJSONError(w, "Vérification Stripe indisponible. Aucun abonnement créé ; recommencez votre demande.", 503)
			return
		}
		writeJSON(w, 200, map[string]string{"checkout_url": checkout})
	}
}

func CustomerCancelSubscriptionHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", 401)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", 400)
			return
		}
		t, err := dbpkg.GetTrial(db, req.ID)
		if err != nil || t.CustomerID != identity.ID {
			writeJSONError(w, "Abonnement introuvable", 404)
			return
		}
		if err = dbpkg.RequestSubscriptionCancellation(db, t.ID, time.Now().UTC()); err != nil {
			writeJSONError(w, "Impossible d'enregistrer la demande", 500)
			return
		}
		if err = processTrialCancellation(db, cfg, t.ID); err != nil {
			writeJSON(w, 202, map[string]string{"message": "Demande enregistrée. Confirmation Stripe en attente : ne considérez pas encore le prélèvement comme annulé. Contactez-nous si l'échéance est proche."})
			return
		}
		writeJSON(w, 200, map[string]string{"message": "Renouvellement automatique annulé. L'accès reste disponible jusqu'à la fin de la période gratuite ou déjà payée."})
	}
}

func processTrialCancellation(db *sql.DB, cfg *config.Config, id string) error {
	t, err := dbpkg.GetTrial(db, id)
	if err != nil {
		return err
	}
	if t.SubscriptionID == "" {
		if t.State == "provisioning" {
			return errors.New("création Stripe en cours, annulation à reprendre")
		}
		_, err = db.Exec(`UPDATE trial_applications SET state='rejected',ended_at=COALESCE(ended_at,?) WHERE id=? AND state IN ('pending','verified')`, time.Now().Unix(), id)
		return err
	}
	sub, err := getSubscription(cfg, t.SubscriptionID)
	if err != nil {
		return err
	}
	if err = syncTrialSubscription(db, cfg, t, sub); err != nil {
		return err
	}
	_, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: id, Kind: "cancelled"}, "trial-cancelled:"+id, 20)
	return err
}

func processTrialEmail(db *sql.DB, cfg *config.Config, m *mailer.Mailer, job *dbpkg.Job) error {
	if job.Type == jobTrialVerify {
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		t, err := dbpkg.GetTrial(db, payload.Value)
		if err != nil {
			return err
		}
		token, err := dbpkg.CreateTrialEmailToken(db, t.ID, time.Now().UTC())
		if errors.Is(err, dbpkg.ErrTrialUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		link := strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/essai/#token=" + token
		return m.SendTrialMessage(t.Email, "Confirmez votre demande d'essai RelaisDesk", "Confirmez votre adresse avant de vérifier votre carte chez Stripe :\n"+link+"\n\nLien à usage unique valable 15 minutes. Aucun essai ni prélèvement n'est créé par ce message. Si vous n'êtes pas à l'origine de la demande, ignorez ce message.\nRéférence de demande : "+t.ID, false)
	}
	var payload trialNotice
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return err
	}
	t, err := dbpkg.GetTrial(db, payload.ID)
	if err != nil {
		return err
	}
	portal := strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/client/"
	period := "mois"
	if t.BillingCycle == "annual" {
		period = "an"
	}
	body := fmt.Sprintf("Offre %s — %d technicien(s) simultané(s).\nAprès l'essai : %.2f EUR/%s, renouvellement et paiement automatiques.\nFin de l'essai / première échéance : %s (UTC).\nGestion et annulation du renouvellement : %s\n", t.Plan, t.Technicians, float64(t.PriceCents)/100, period, time.Unix(t.TrialEnd, 0).UTC().Format("02/01/2006 à 15:04"), portal)
	subject := "Votre essai RelaisDesk"
	switch payload.Kind {
	case "activated":
		if t.WithdrawalImmediate {
			return nil
		}
		lic, err := dbpkg.GetLicense(db, t.LicenseID)
		if err != nil {
			return err
		}
		technicianKey := payload.LicenseKey
		if technicianKey == "" {
			// Key already delivered by the first activation (replay):
			// never print the stored hash, point to support instead.
			technicianKey = lic.KeyHint + " (clé envoyée lors de l'activation ; contactez-nous en cas de perte)"
		}
		body = "Votre essai gratuit de 30 jours est activé.\n" + body + fmt.Sprintf("\nLicence : %s\nClé technicien : %s\n\nPour accéder à l'espace client, utilisez « Mot de passe oublié » afin de créer votre mot de passe. Annulez avant l'échéance pour ne pas être prélevé.\n", lic.LicenseID, technicianKey)
		body += "\nRéférence du contrat : " + t.ID + "\nRétractation du contrat (distincte de l'annulation des prochains renouvellements) : " + strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/formulaire-retractation.html#trial=" + t.ID + "\nConsommateurs : délai légal de 14 jours à compter de la conclusion du contrat, sans renonciation par l'activation immédiate ; RelaisDesk accepte en outre la rétractation sans frais pendant l'intégralité de l'essai gratuit.\n"
	case "reminder":
		if t.CancelRequestedAt != 0 || t.CancelAtPeriodEnd || t.StripeStatus != "trialing" || time.Now().Unix() >= t.TrialEnd {
			return nil
		}
		subject = "Votre essai RelaisDesk se termine prochainement"
		body = "Rappel avant le premier prélèvement automatique.\n" + body
	case "nurture":
		if t.CancelRequestedAt != 0 || t.CancelAtPeriodEnd || t.StripeStatus != "trialing" || time.Now().Unix() >= t.TrialEnd {
			return nil
		}
		subject = "Bien démarrer avec votre essai RelaisDesk"
		site := strings.TrimRight(cfg.PublicWebsiteURL, "/")
		body = "Vous testez RelaisDesk depuis quelques jours : voici l'essentiel pour en tirer le meilleur.\n\n" +
			"1. Connectez votre premier poste depuis votre console technicien.\n" +
			"2. Faites un test d'accès permanent sur une machine supervisée.\n" +
			"3. Lisez le guide de démarrage : " + site + "/guide-demarrage.html\n\n" +
			"Une question ? Répondez à cet email, on vous répond vite.\n" + body
	case "reminder_final":
		if t.CancelRequestedAt != 0 || t.CancelAtPeriodEnd || t.StripeStatus != "trialing" || time.Now().Unix() >= t.TrialEnd {
			return nil
		}
		subject = "Plus que quelques jours d'essai RelaisDesk"
		body = "Dernier rappel : sans action de votre part, le premier prélèvement automatique interviendra à l'échéance. " +
			"Pour ne pas être prélevé, annulez le renouvellement depuis votre espace client avant cette date.\n" + body
	case "cancelled":
		subject = "Confirmation d'annulation du renouvellement RelaisDesk"
		end := t.TrialEnd
		if t.PaidThrough > end {
			end = t.PaidThrough
		}
		body = fmt.Sprintf("Votre renouvellement automatique est annulé.\nContrat : %s — offre %s — licence %s.\nFin de l'accès gratuit ou payé : %s (UTC). Aucun nouveau cycle ne sera souscrit. Les éventuels paiements déjà engagés ne sont pas automatiquement remboursés ; vos droits légaux restent applicables.\nGérer votre compte : %s", t.ID, t.Plan, t.LicenseID, time.Unix(end, 0).UTC().Format("02/01/2006 à 15:04"), portal)
	case "payment_failed":
		subject = "Paiement de votre abonnement RelaisDesk à régulariser"
		body = "Le dernier prélèvement n'a pas abouti. L'accès n'est pas prolongé sans paiement et aucun nouvel essai n'est accordé. Mettez à jour votre carte bancaire depuis votre espace client (bouton « Gérer mon moyen de paiement ») ou contactez-nous. Vous pouvez également annuler les prochains renouvellements depuis votre espace client.\n" + portal
	case "renewal_paused":
		subject = "Votre reconduction annuelle RelaisDesk est suspendue"
		body = "Nous n'avons pas pu confirmer l'envoi de l'information annuelle dans le délai prévu. Pour éviter une reconduction non annoncée, nous avons demandé l'arrêt du prochain renouvellement. Votre accès à la période déjà payée est conservé. Une confirmation de l'arrêt Stripe suivra. Contactez-nous pour poursuivre le service au-delà de l'échéance, sans nouvelle période d'essai.\n" + portal
	case "withdrawal_payment":
		if err := m.SendWithdrawalNotificationEmail("PAIEMENT-APRES-RETRACTATION", t.ID, t.Email, t.Billing.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		subject = "Paiement à régulariser après votre rétractation RelaisDesk"
		body = "Un paiement a été confirmé après votre notification de rétractation de l'essai. Il ne réactive pas votre licence. Nous avons enregistré cette anomalie pour remboursement et traitement comptable ; aucun service gratuit ne sera facturé rétroactivement.\nContrat : " + t.ID + "\nContact : " + strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/contact.html"
	case "rejected":
		subject = "Votre demande d'essai RelaisDesk"
		body = "L'essai n'a pas été ouvert : la demande a expiré ou l'éligibilité n'a pas pu être confirmée. L'offre est réservée aux nouveaux clients n'en ayant pas déjà bénéficié. Aucun abonnement ni prélèvement n'a été créé pour cette demande. Si vous pensez qu'il s'agit d'une erreur (carte professionnelle partagée, par exemple), contactez-nous via le formulaire du site pour un examen humain.\n" + strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/contact.html"
	default:
		return errors.New("notification d'essai inconnue")
	}
	if err := m.SendTrialMessage(t.Email, subject, body, payload.Kind == "activated", t.Billing.TermsVersion); err != nil {
		return err
	}
	if payload.Kind == "activated" && payload.LicenseKey != "" {
		// The key was delivered: scrub it from the retained job row so a
		// database read never yields it. A scrub failure must not retry
		// (the email is sent); it is logged for operator follow-up.
		redacted, _ := json.Marshal(trialNotice{ID: payload.ID, Kind: payload.Kind})
		if err := dbpkg.ScrubJobPayload(db, job.ID, string(redacted)); err != nil {
			log.Printf("[Jobs] purge de la clé du travail #%d impossible: %v", job.ID, err)
		}
	}
	return nil
}

// Reuses the hourly maintenance pass; no additional monitoring service.
func QueueTrialReminders(db *sql.DB, now time.Time) error {
	rows, err := db.Query(`SELECT id FROM trial_applications WHERE state='active' AND stripe_status='trialing' AND cancel_requested_at IS NULL AND cancel_at_period_end=0 AND trial_end>? AND trial_end<=?`, now.Unix(), now.Add(7*24*time.Hour).Unix())
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: id, Kind: "reminder"}, "trial-reminder:"+id, 20); err != nil {
			return err
		}
	}
	return nil
}

// QueueTrialNurture enqueues the J+3 onboarding email (trial started 3-7 days
// ago, still trialing) and the final reminder (trial ending within 3 days).
// Dedup keys make the hourly pass idempotent.
func QueueTrialNurture(db *sql.DB, now time.Time) error {
	now = now.UTC()
	queries := []struct {
		kind  string
		key   string
		query string
		args  []any
	}{
		{"nurture", "trial-nurture:",
			`SELECT id FROM trial_applications WHERE state='active' AND stripe_status='trialing' AND cancel_requested_at IS NULL AND cancel_at_period_end=0 AND trial_start<=? AND trial_start>? AND trial_end>?`,
			[]any{now.Add(-3 * 24 * time.Hour).Unix(), now.Add(-7 * 24 * time.Hour).Unix(), now.Unix()}},
		{"reminder_final", "trial-reminder-final:",
			`SELECT id FROM trial_applications WHERE state='active' AND stripe_status='trialing' AND cancel_requested_at IS NULL AND cancel_at_period_end=0 AND trial_end>? AND trial_end<=?`,
			[]any{now.Unix(), now.Add(3 * 24 * time.Hour).Unix()}},
	}
	for _, q := range queries {
		rows, err := db.Query(q.query, q.args...)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: id, Kind: q.kind}, q.key+id, 20); err != nil {
				return err
			}
		}
	}
	return nil
}

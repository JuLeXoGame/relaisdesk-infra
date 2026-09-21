package handlers

import (
	"api/config"
	"api/mailer"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const jobSubscriptionRenewalNotice = "subscription_renewal_notice"

type subscriptionRenewalNotice struct {
	ID  string `json:"id"`
	End int64  `json:"end"`
}

// One annual-cycle notice at J60. Monthly subscriptions are indefinite-term
// contracts billed monthly, not successive fixed-term contracts. If an annual
// notice cannot be recorded by J35 (margin over a full calendar month), turn
// off renewal instead of silently billing an unannounced annual commitment.
func QueueSubscriptionRenewalNotices(db *sql.DB, now time.Time) error {
	rows, err := db.Query(`SELECT id,paid_through FROM trial_applications WHERE state='active' AND billing_cycle='annual'
        AND stripe_status NOT IN ('canceled','incomplete_expired') AND cancel_requested_at IS NULL AND cancel_at_period_end=0
        AND paid_through>0 AND paid_through<=?`, now.Add(60*24*time.Hour).Unix())
	if err != nil {
		return err
	}
	items := []subscriptionRenewalNotice{}
	for rows.Next() {
		var p subscriptionRenewalNotice
		if err = rows.Scan(&p.ID, &p.End); err != nil {
			rows.Close()
			return err
		}
		items = append(items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range items {
		var sent int
		if err = db.QueryRow(`SELECT COUNT(*) FROM subscription_renewal_notices WHERE application_id=? AND period_end=?`, p.ID, p.End).Scan(&sent); err != nil {
			return err
		}
		if sent != 0 {
			continue
		}
		if p.End <= now.Add(35*24*time.Hour).Unix() {
			if err = dbpkg.RequestSubscriptionCancellation(db, p.ID, now); err != nil {
				return err
			}
			if _, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: p.ID, Kind: "renewal_paused"}, fmt.Sprintf("renewal-paused:%s:%d", p.ID, p.End), 20); err != nil {
				return err
			}
			continue
		}
		if _, err = enqueueJSONJob(db, jobSubscriptionRenewalNotice, p, fmt.Sprintf("annual-notice:%s:%d", p.ID, p.End), 30); err != nil {
			return err
		}
	}
	return nil
}

func processSubscriptionRenewalNotice(db *sql.DB, cfg *config.Config, m *mailer.Mailer, job *dbpkg.Job) error {
	var p subscriptionRenewalNotice
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return err
	}
	t, err := dbpkg.GetTrial(db, p.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if t.BillingCycle != "annual" || t.PaidThrough != p.End || t.CancelRequestedAt != 0 || t.CancelAtPeriodEnd || t.StripeStatus == "canceled" {
		return nil
	}
	if p.End <= now.Add(35*24*time.Hour).Unix() {
		return QueueSubscriptionRenewalNotices(db, now)
	}
	if p.End > now.Add(90*24*time.Hour).Unix() {
		return nil
	}
	var sent int
	if err = db.QueryRow(`SELECT COUNT(*) FROM subscription_renewal_notices WHERE application_id=? AND period_end=?`, t.ID, p.End).Scan(&sent); err != nil {
		return err
	}
	if sent != 0 {
		return nil
	}
	sub, err := getSubscription(cfg, t.SubscriptionID)
	if err != nil {
		return err
	}
	if err = syncTrialSubscription(db, cfg, t, sub); err != nil {
		return err
	}
	if sub.CancelAtPeriodEnd || sub.Status == "canceled" {
		return nil
	}
	if !subscriptionMatchesTrial(sub, t) || sub.CurrentPeriodEnd != p.End {
		return fmt.Errorf("échéance annuelle Stripe différente du contrat payé")
	}
	body := fmt.Sprintf("INFORMATION PERSONNELLE — RECONDUCTION ANNUELLE RELAISDESK\n\nContrat : %s\nLicence : %s\nOffre : %s — %d technicien(s) simultané(s)\n\n┌────────────────────────────────────────────────────────────\n│ DATE LIMITE POUR REFUSER LA RECONDUCTION\n│ Avant le %s (UTC)\n└────────────────────────────────────────────────────────────\n\nVous pouvez refuser sans frais la prochaine reconduction depuis votre espace client : %s/client/\nRubrique Abonnements, Annuler le renouvellement, puis confirmation. Vous conservez l'accès jusqu'à l'échéance de la période payée.\n\nSans annulation, votre abonnement sera renouvelé pour un an à cette échéance, au prix de %.2f EUR prélevé en une seule fois. TVA non applicable, art. 293 B du CGI.\nEn cas de difficulté : %s/contact.html ou 06 62 85 59 30. Vos droits légaux restent applicables.\n\nJulien BELLOT EI / RelaisDesk\nSIRET 94074710800014\n2 Chemin de Lonzais, 03170 Bizeneuille, France", t.ID, t.LicenseID, t.Plan, t.Technicians, time.Unix(p.End, 0).UTC().Format("02/01/2006 à 15:04:05"), strings.TrimRight(cfg.PublicWebsiteURL, "/"), float64(t.PriceCents)/100, strings.TrimRight(cfg.PublicWebsiteURL, "/"))
	if err = m.SendTrialMessage(t.Email, "RelaisDesk : votre droit de refuser la reconduction annuelle", body, false); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO subscription_renewal_notices(application_id,period_end,sent_at,notice_text) VALUES(?,?,?,?) ON CONFLICT(application_id,period_end) DO NOTHING`, t.ID, p.End, now.Unix(), body)
	return err
}

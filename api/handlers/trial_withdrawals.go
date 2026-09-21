package handlers

import (
	"api/config"
	"api/mailer"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const jobTrialWithdrawalProcess = "trial_withdrawal_process"
const jobTrialWithdrawalEmail = "trial_withdrawal_email"

func PublicTrialWithdrawalHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", 400)
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		parsed, err := mail.ParseAddress(req.Email)
		if err != nil || parsed.Address != req.Email || strings.ContainsAny(req.Email, "\r\n\t") {
			writeJSONError(w, "Adresse e-mail invalide", 400)
			return
		}
		item, err := dbpkg.RequestTrialWithdrawal(db, req.ID, req.Email, req.Name, time.Now().UTC())
		if err != nil {
			writeJSONError(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, map[string]any{"request_id": item.RequestID, "contract_id": item.ApplicationID, "requested_at": time.Unix(item.RequestedAt, 0).UTC().Format(time.RFC3339), "status": item.Status, "message": "Notification de rétractation enregistrée à cette date. Conservez cet accusé de réception. Le traitement de l'abonnement et de tout remboursement éventuel est distinct ; une confirmation suivra par e-mail."})
	}
}

func processTrialWithdrawal(db *sql.DB, cfg *config.Config, id string) error {
	w, err := dbpkg.GetTrialWithdrawal(db, id)
	if err != nil {
		return err
	}
	t, err := dbpkg.GetTrial(db, id)
	if err != nil {
		return err
	}
	if t.SubscriptionID == "" {
		if t.State == "provisioning" {
			return errors.New("rétractation en attente de reprise de la création Stripe")
		}
		if _, err = db.Exec(`UPDATE trial_applications SET state='rejected',ended_at=COALESCE(ended_at,?) WHERE id=? AND state IN ('pending','verified')`, time.Now().Unix(), id); err != nil {
			return err
		}
	} else {
		sub, err := getSubscription(cfg, t.SubscriptionID)
		if err != nil {
			return err
		}
		if err = syncTrialSubscription(db, cfg, t, sub); err != nil {
			return err
		}
	}
	status := "manual_review"
	if w.Immediate {
		status = "completed"
	}
	_, err = db.Exec(`UPDATE trial_withdrawals SET processed_at=COALESCE(processed_at,?),status=CASE WHEN status='refund_review' THEN status ELSE ? END WHERE application_id=?`, time.Now().Unix(), status, id)
	return err
}

func processTrialWithdrawalEmail(db *sql.DB, cfg *config.Config, m *mailer.Mailer, job *dbpkg.Job) error {
	var p stringPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return err
	}
	w, err := dbpkg.GetTrialWithdrawal(db, p.Value)
	if err != nil {
		return err
	}
	t, err := dbpkg.GetTrial(db, p.Value)
	if err != nil {
		return err
	}
	date := time.Unix(w.RequestedAt, 0).UTC().Format(time.RFC3339)
	if w.CustomerEmailSentAt == 0 {
		body := fmt.Sprintf("Votre notification de rétractation est reçue.\nRéférence : %s\nContrat : %s\nNom déclaré : %s\nRéception (UTC) : %s\n\nCette date fait foi pour la réception, indépendamment du délai de traitement technique. Aucun nouvel essai n'est accordé. Vous recevrez une confirmation distincte de l'arrêt Stripe ; un éventuel remboursement sera traité selon vos droits. Conservez ce message.\nContact : %s/contact.html", w.RequestID, t.ID, w.CustomerName, date, strings.TrimRight(cfg.PublicWebsiteURL, "/"))
		if err = m.SendTrialMessage(t.Email, "Accusé de réception de votre rétractation RelaisDesk", body, false); err != nil {
			return err
		}
		if _, err = db.Exec(`UPDATE trial_withdrawals SET customer_email_sent_at=? WHERE application_id=?`, time.Now().Unix(), t.ID); err != nil {
			return err
		}
	}
	if w.AdminEmailSentAt == 0 {
		if err = m.SendWithdrawalNotificationEmail(w.RequestID, "ESSAI "+t.ID, t.Email, w.CustomerName, date); err != nil {
			return err
		}
		if _, err = db.Exec(`UPDATE trial_withdrawals SET admin_email_sent_at=? WHERE application_id=?`, time.Now().Unix(), t.ID); err != nil {
			return err
		}
	}
	if w.ProcessedAt == 0 {
		return errors.New("confirmation de traitement de rétractation en attente")
	}
	message := "Votre demande a été enregistrée et les prochaines reconductions ont été arrêtées. L'examen de la rétractation et d'un éventuel remboursement se poursuit ; vos droits ne sont pas refusés au seul motif de la date de demande."
	if w.Immediate {
		message = "Votre rétractation pendant l'essai gratuit est traitée : l'accès à cette licence est arrêté et l'abonnement Stripe est résilié, sans facturation de l'essai. Tout débit qui aurait été engagé entre-temps doit nous être signalé pour remboursement."
	}
	return m.SendTrialMessage(t.Email, "Suite de votre rétractation RelaisDesk", message+"\nRéférence : "+w.RequestID+"\nContrat : "+t.ID, false)
}

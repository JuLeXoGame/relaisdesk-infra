package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	netmail "net/mail"
	"strings"

	"api/config"
	"api/mailer"
	"api/middleware"
)

type ContactRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
	Honeypot string `json:"website"` // Honeypot field - must be empty
}

// PublicContactHandler handles contact form submissions from the website.
func PublicContactHandler(mail *mailer.Mailer, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
			return
		}

		var req ContactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		// Honeypot anti-spam check: if filled, bots fell for the trap
		if strings.TrimSpace(req.Honeypot) != "" {
			log.Printf("[SPAM BOT DETECTED] Honeypot filled from IP %s", middleware.GetClientIP(r))
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "sent",
				"message": "Votre message a bien été envoyé. Nous vous répondrons dans les plus brefs délais.",
			})
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" || len(name) > 100 {
			writeJSONError(w, "Veuillez renseigner votre nom (maximum 100 caractères).", http.StatusBadRequest)
			return
		}

		email := strings.TrimSpace(req.Email)
		parsedEmail, err := netmail.ParseAddress(email)
		if err != nil || parsedEmail.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n\t") {
			writeJSONError(w, "Veuillez renseigner une adresse email valide.", http.StatusBadRequest)
			return
		}

		subject := strings.TrimSpace(req.Subject)
		if len(subject) > 200 {
			writeJSONError(w, "L'objet est trop long (maximum 200 caractères).", http.StatusBadRequest)
			return
		}
		if subject == "" {
			subject = "Demande d'information"
		}

		message := strings.TrimSpace(req.Message)
		if len(message) < 5 {
			writeJSONError(w, "Le message est trop court (minimum 5 caractères).", http.StatusBadRequest)
			return
		}
		if len(message) > 5000 {
			writeJSONError(w, "Le message est trop long (maximum 5000 caractères).", http.StatusBadRequest)
			return
		}

		clientIP := middleware.GetClientIP(r)
		log.Printf("[CONTACT FORM] Message reçu de %s (%s) depuis IP %s — Objet: %s", name, email, clientIP, subject)

		if mail == nil || !mail.IsConfigured() {
			log.Printf("[CONTACT FORM] SMTP non configuré, impossible d'acheminer le message de %s", email)
			writeJSONError(w, "Le service d'envoi d'emails est momentanément indisponible. Veuillez réessayer ultérieurement.", http.StatusServiceUnavailable)
			return
		}

		if err := mail.SendContactEmail(name, email, subject, message); err != nil {
			log.Printf("[CONTACT FORM] Échec envoi email de contact: %v", err)
			writeJSONError(w, "Erreur lors de l'envoi du message. Veuillez réessayer ultérieurement.", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "sent",
			"message": "Votre message a bien été envoyé. Nous vous répondrons dans les plus brefs délais.",
		})
	}
}

package mailer

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const CurrentTermsVersion = "2026-09-27"
const CurrentTrialTermsVersion = "2026-09-24-fleet-v3"
const contractualTermsFilename = "CGV-RelaisDesk-" + CurrentTermsVersion + ".txt"

// contractualTerms is an immutable copy of the conditions accepted at checkout.
//
//go:embed legal/CGV-RelaisDesk-2026-09-27.txt
var contractualTerms []byte

// Keep accepted 2026-09-21 contracts byte-for-byte, including their older terms.
//
//go:embed legal/CGV-RelaisDesk-2026-09-21.txt
var september21ContractualTerms []byte

// Keep accepted 2026-09-11 contracts byte-for-byte, including their older terms.
//
//go:embed legal/CGV-RelaisDesk-2026-09-11.txt
var september11ContractualTerms []byte

// Keep accepted fleet-v1 contracts byte-for-byte, including their older DPA.
//
//go:embed legal/CGV-RelaisDesk-2026-09-10.txt
var september10ContractualTerms []byte

//go:embed legal/CGV-RelaisDesk-2026-09-09.txt
var september9ContractualTerms []byte

//go:embed legal/ESSAI-RelaisDesk-2026-09-24-fleet-v3.txt
var fleetTrialSupplement []byte

// Keep the accepted 2026-09-21 trial supplement byte-for-byte.
//
//go:embed legal/ESSAI-RelaisDesk-2026-09-21-fleet-v2.txt
var fleetTrialSupplementV2 []byte

//go:embed legal/ESSAI-RelaisDesk-2026-09-11-fleet-v2.txt
var september11TrialSupplement []byte

//go:embed legal/ESSAI-RelaisDesk-2026-09-10-fleet-v1.txt
var september10TrialSupplement []byte

//go:embed legal/CGV-RelaisDesk-2026-09-08.txt
var september8ContractualTerms []byte

//go:embed legal/ESSAI-RelaisDesk-2026-09-09.txt
var trialSupplement []byte

//go:embed legal/ESSAI-RelaisDesk-2026-09-09-v2.txt
var trialSupplementV2 []byte

// Preserve the previous price grid for orders already accepted at that price.
//
//go:embed legal/CGV-RelaisDesk-2026-09-07.txt
var september7ContractualTerms []byte

// Preserve the conditions of pending orders accepted before the update.
//
//go:embed legal/CGV-RelaisDesk-2026-08-25.txt
var previousContractualTerms []byte

func termsAttachment(versions ...string) (string, []byte, error) {
	if len(versions) > 1 {
		return "", nil, fmt.Errorf("une seule version contractuelle est attendue")
	}
	if len(versions) == 0 || versions[0] == "" {
		// Legacy/manual orders have no recorded acceptance: do not invent one.
		return "", nil, nil
	}
	switch versions[0] {
	case CurrentTrialTermsVersion:
		combined := append(append([]byte{}, contractualTerms...), fleetTrialSupplement...)
		return "CGV-et-ESSAI-RelaisDesk-" + CurrentTrialTermsVersion + ".txt", combined, nil
	case "2026-09-21-fleet-v2":
		combined := append(append([]byte{}, september21ContractualTerms...), fleetTrialSupplementV2...)
		return "CGV-et-ESSAI-RelaisDesk-2026-09-21-fleet-v2.txt", combined, nil
	case "2026-09-11-fleet-v2":
		combined := append(append([]byte{}, september11ContractualTerms...), september11TrialSupplement...)
		return "CGV-et-ESSAI-RelaisDesk-2026-09-11-fleet-v2.txt", combined, nil
	case "2026-09-10-fleet-v1":
		combined := append(append([]byte{}, september10ContractualTerms...), september10TrialSupplement...)
		return "CGV-et-ESSAI-RelaisDesk-2026-09-10-fleet-v1.txt", combined, nil
	case "2026-09-09-trial-v2":
		combined := append(append([]byte{}, september9ContractualTerms...), trialSupplementV2...)
		return "CGV-et-ESSAI-RelaisDesk-2026-09-09-v2.txt", combined, nil
	case "2026-09-09-trial":
		combined := append(append([]byte{}, september8ContractualTerms...), trialSupplement...)
		return "CGV-et-ESSAI-RelaisDesk-2026-09-09.txt", combined, nil
	case CurrentTermsVersion:
		return contractualTermsFilename, contractualTerms, nil
	case "2026-09-21":
		return "CGV-RelaisDesk-2026-09-21.txt", september21ContractualTerms, nil
	case "2026-09-11":
		return "CGV-RelaisDesk-2026-09-11.txt", september11ContractualTerms, nil
	case "2026-09-10":
		return "CGV-RelaisDesk-2026-09-10.txt", september10ContractualTerms, nil
	case "2026-09-09":
		return "CGV-RelaisDesk-2026-09-09.txt", september9ContractualTerms, nil
	case "2026-09-08":
		return "CGV-RelaisDesk-2026-09-08.txt", september8ContractualTerms, nil
	case "2026-09-07":
		return "CGV-RelaisDesk-2026-09-07.txt", september7ContractualTerms, nil
	case "2026-08-25":
		return "CGV-RelaisDesk-2026-08-25.txt", previousContractualTerms, nil
	default:
		return "", nil, fmt.Errorf("version contractuelle non archivée")
	}
}

// Trial confirmations include the base CGV and the explicitly accepted
// recurring-subscription supplement, without changing historical contracts.
func (m *Mailer) SendTrialMessage(to, subject, body string, attachTerms bool, versions ...string) error {
	if !attachTerms {
		return m.sendRawEmail(to, subject, body, "", nil)
	}
	version := CurrentTrialTermsVersion
	if len(versions) > 0 {
		version = versions[0]
	}
	name, contents, err := termsAttachment(version)
	if err != nil {
		return err
	}
	return m.sendRawEmail(to, subject, body, name, contents)
}

// Mailer handles sending emails via SMTP (Port 587 STARTTLS or Port 465 TLS direct).
type Mailer struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// NewMailer creates a new Mailer instance.
func NewMailer(host string, port int, user, password, from string) *Mailer {
	return &Mailer{
		Host:     strings.TrimSpace(host),
		Port:     port,
		User:     strings.TrimSpace(user),
		Password: password,
		From:     strings.TrimSpace(from),
	}
}

// IsConfigured returns true if SMTP credentials and host are provided.
func (m *Mailer) IsConfigured() bool {
	if m == nil || m.Host == "" || strings.ContainsAny(m.Host, "\r\n\t /:") ||
		(m.Port != 465 && m.Port != 587) || m.From == "" || ((m.User == "") != (m.Password == "")) {
		return false
	}
	from, err := mail.ParseAddress(m.From)
	return err == nil && from.Address != ""
}

// SendTestEmail sends a test email to verify the SMTP connection and credentials.
func (m *Mailer) SendTestEmail(toEmail string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("SMTP non configuré : veuillez définir SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS et SMTP_FROM dans votre fichier .env")
	}

	target := strings.TrimSpace(toEmail)
	if target == "" {
		target = m.From
	}

	subject := "RelaisDesk — Test de connexion SMTP réussi ✅"
	bodyText := fmt.Sprintf(`Bonjour,

Ce message confirme que la configuration SMTP de votre serveur RelaisDesk est 100%% opérationnelle !

Détails de la connexion :
--------------------------------------------------
Serveur SMTP       : %s
Port               : %d (%s)
Utilisateur        : %s
Expéditeur (From)  : %s
Destinataire (To)  : %s
Horodatage         : %s
--------------------------------------------------

Le pipeline d'envoi d'emails transactionnels (licences, factures PDF en pièce jointe) est prêt.

L'équipe RelaisDesk
https://relaisdesk.fr
`, m.Host, m.Port, m.getPortDescription(), m.User, m.From, target, time.Now().Format("2006-01-02 15:04:05 MST"))

	return m.sendRawEmail(target, subject, bodyText, "", nil)
}

// SendLicenseEmail sends an email containing the license details to the technician.
func (m *Mailer) SendLicenseEmail(toEmail, planName, licenseID, licenseKey, expiresAt string, technicians int, termsVersion ...string) error {
	filename, terms, err := termsAttachment(termsVersion...)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Votre licence RelaisDesk (%s)", planName)

	bodyText := fmt.Sprintf(`Bonjour,

Merci pour votre souscription à RelaisDesk !

Voici les informations de votre licence professionnelle :
--------------------------------------------------
Offre                  : %s
Connexions simultanées : %d technicien(s)
Identifiant Licence    : %s
Clé de Licence         : %s
Date d'expiration      : %s
--------------------------------------------------

Comment démarrer :
1. Téléchargez et installez l'application Technicien (Installeur Windows) :
   https://api.relaisdesk.fr/api/v1/downloads/configurator-setup
   (Ou version portable : https://api.relaisdesk.fr/api/v1/downloads/configurator)
2. Lancez l'application et saisissez votre ID et votre Clé de Licence.
3. Vos clients peuvent télécharger le Viewer gratuit :
   https://api.relaisdesk.fr/api/v1/downloads/viewer

Pour toute question ou assistance : contact@relaisdesk.fr — 06 62 85 59 30
Logiciel libre et code source : https://relaisdesk.fr/logiciel-libre.html

L'équipe RelaisDesk
https://relaisdesk.fr
`, planName, technicians, licenseID, licenseKey, expiresAt)

	return m.sendRawEmail(toEmail, subject, bodyText, filename, terms)
}

// SendCustomerLoginLink sends a short-lived, single-use magic link. The token
// is never written to application logs or stored in plaintext in SQLite.
func (m *Mailer) SendCustomerLoginLink(toEmail, loginURL string, validFor time.Duration) error {
	subject := "Votre lien de connexion à l'espace client RelaisDesk"
	bodyText := fmt.Sprintf(`Bonjour,

Vous avez demandé l'accès à votre espace client commercial RelaisDesk.

Lien de connexion à usage unique (valable %d minutes) :
%s

Si vous n'êtes pas à l'origine de cette demande, ignorez simplement ce message. Ne transférez pas ce lien.

L'équipe RelaisDesk
https://relaisdesk.fr
`, int(validFor.Minutes()), loginURL)
	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

// SendCustomer2FACode sends a 6-digit numeric verification code for 2FA login.
func (m *Mailer) SendCustomer2FACode(toEmail, code string, validFor time.Duration) error {
	subject := "Votre code de sécurité RelaisDesk : " + code
	bodyText := fmt.Sprintf(`Bonjour,

Voici votre code d'authentification pour vous connecter à votre compte RelaisDesk :

    %s

Ce code est valable pendant %d minutes.

Ne transmettez jamais ce code. Si vous n'êtes pas à l'origine de cette tentative de connexion, nous vous recommandons de modifier votre mot de passe immédiatement.

L'équipe RelaisDesk
https://relaisdesk.fr
`, code, int(validFor.Minutes()))
	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

func (m *Mailer) SendTeamInvitation(toEmail, inviteURL string) error {
	body := fmt.Sprintf("Bonjour,\n\nLe propriétaire d’une licence RelaisDesk vous invite dans son équipe de techniciens. Après acceptation, vous pourrez accéder uniquement aux dossiers qu’il vous autorise.\n\nAccepter l’invitation (48 heures maximum après sa création, usage unique) :\n%s\n\nVotre compte reste personnel. Si vous avez déjà un compte, sa double authentification reste nécessaire. Ne transmettez jamais ce lien. Si cette invitation n’est pas attendue, ignorez-la.\n\nRelaisDesk\nhttps://relaisdesk.fr\n", inviteURL)
	return m.sendRawEmail(toEmail, "Invitation à une équipe RelaisDesk", body, "", nil)
}

func (m *Mailer) SendRenewalReminder(toEmail, licenseID, expiresAt, portalURL, reminderType string) error {
	subject := "Échéance de votre licence RelaisDesk"
	if reminderType == "expired" {
		subject = "Votre licence RelaisDesk est arrivée à échéance"
	}
	bodyText := fmt.Sprintf(`Bonjour,

Votre licence RelaisDesk %s arrive à échéance ou est arrivée à échéance le %s.

Vous pouvez préparer son renouvellement depuis votre espace client sécurisé :
%s

La licence existante et ses identifiants seront conservés. Aucun prélèvement récurrent n'est déclenché par ce message : le renouvellement nécessite votre validation explicite.

L'équipe RelaisDesk
contact@relaisdesk.fr — 06 62 85 59 30
`, licenseID, expiresAt, portalURL)
	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

func (m *Mailer) SendRenewalConfirmation(toEmail, planName, licenseID, expiresAt string, technicians int, termsVersion ...string) error {
	filename, terms, err := termsAttachment(termsVersion...)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Renouvellement confirmé — licence %s", licenseID)
	bodyText := fmt.Sprintf(`Bonjour,

Le renouvellement de votre licence RelaisDesk a été confirmé.

Offre                  : %s
Licence                : %s
Connexions simultanées : %d
Nouvelle échéance      : %s

Vos identifiants de licence restent inchangés. Votre facture est disponible dans votre espace client et vous est également adressée par e-mail.

L'équipe RelaisDesk
https://relaisdesk.fr/client/
`, planName, licenseID, technicians, expiresAt)
	return m.sendRawEmail(toEmail, subject, bodyText, filename, terms)
}

// SendBankTransferInstructionsEmail sends bank transfer payment details to the customer.
func (m *Mailer) SendBankTransferInstructionsEmail(toEmail, orderID, planName string, amount float64, iban, bic, holder string, termsVersion ...string) error {
	filename, terms, err := termsAttachment(termsVersion...)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Instructions de virement pour votre commande RelaisDesk (%s)", orderID)

	bodyText := fmt.Sprintf(`Bonjour,

Nous avons bien enregistré votre commande %s pour le plan %s.

Pour activer votre licence, veuillez effectuer le virement bancaire suivant :
--------------------------------------------------
Montant              : %.2f €
Référence OBLIGATOIRE: %s
Bénéficiaire         : %s
IBAN                 : %s
BIC                  : %s
--------------------------------------------------

IMPORTANT : Veuillez impérativement reporter la référence "%s" dans le libellé de votre virement pour que votre compte soit activé dès réception.

L'équipe RelaisDesk
https://relaisdesk.fr
`, orderID, planName, amount, orderID, holder, iban, bic, orderID)

	return m.sendRawEmail(toEmail, subject, bodyText, filename, terms)
}

// SendCartReminderEmail nudges the customer to complete an unpaid Stripe
// order. Round 2 is the last call before the 48h order expiry.
func (m *Mailer) SendCartReminderEmail(toEmail, orderID, planName string, amount float64, expiresAt, resumeURL string, round int) error {
	subject := fmt.Sprintf("Votre commande RelaisDesk %s vous attend", orderID)
	intro := "Vous n'avez pas finalisé votre commande."
	if round >= 2 {
		subject = fmt.Sprintf("Dernière chance : votre commande RelaisDesk %s expire bientôt", orderID)
		intro = "Dernier rappel avant expiration de votre commande."
	}
	expiry := ""
	if strings.TrimSpace(expiresAt) != "" {
		expiry = fmt.Sprintf("\nVotre commande expire le %s (UTC), après quoi il faudra la recommencer.\n", expiresAt)
	}
	bodyText := fmt.Sprintf(`Bonjour,

%s

Référence : %s
Offre     : %s
Montant   : %.2f €
%s
Finalisez votre commande en 2 minutes (carte bancaire, sans engagement) :
%s

Si vous avez déjà payé ou si vous rencontrez un problème, répondez simplement à cet email.

L'équipe RelaisDesk
https://relaisdesk.fr
`, intro, orderID, planName, amount, expiry, resumeURL)

	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

// SendCryptoInstructionsEmail sends the payment recap (exact amount, deposit
// address, destination tag, expiry) so the customer can pay even after
// closing the page. destTag is nil for BTC.
func (m *Mailer) SendCryptoInstructionsEmail(toEmail, orderID, planName string, amountEUR float64, asset, amountCrypto, rateEUR, payAddress string, destTag *uint32, expiresAt time.Time, termsVersion ...string) error {
	filename, terms, err := termsAttachment(termsVersion...)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("Instructions de paiement %s pour votre commande RelaisDesk (%s)", asset, orderID)

	tagLine := ""
	if destTag != nil {
		tagLine = fmt.Sprintf("Tag de dépôt (Destination Tag) : %d — À REPRENDRE EXACTEMENT\n", *destTag)
	}
	bodyText := fmt.Sprintf(`Bonjour,

Nous avons bien enregistré votre commande %s pour le plan %s.

Pour activer votre licence, réglez exactement %s %s à l'adresse suivante avant le %s :
--------------------------------------------------
Montant en %s : %s %s
Adresse de dépôt : %s
%sCours appliqué : %s EUR/%s (OKX, garanti jusqu'à expiration du devis)
Montant en euros (seul montant faisant foi) : %.2f €
--------------------------------------------------

IMPORTANT : réglez le montant exact avant l'expiration du devis ; les frais du réseau s'ajoutent au montant demandé. Passé ce délai, demandez un nouveau devis depuis le récapitulatif de commande (référence %s). Votre licence est activée automatiquement à réception du dépôt.

L'équipe RelaisDesk
https://relaisdesk.fr
`, orderID, planName, amountCrypto, asset, expiresAt.Format("02/01/2006 15:04"),
		asset, amountCrypto, asset, payAddress, tagLine, rateEUR, asset, amountEUR, orderID)

	return m.sendRawEmail(toEmail, subject, bodyText, filename, terms)
}

// SendInvoiceEmailWithPDF sends invoice confirmation to the customer with the PDF file as MIME attachment.
func (m *Mailer) SendInvoiceEmailWithPDF(toEmail, invoiceNumber, planName string, amount float64, pdfBytes []byte) error {
	subject := fmt.Sprintf("Votre facture RelaisDesk (%s)", invoiceNumber)

	bodyText := fmt.Sprintf(`Bonjour,

Veuillez trouver ci-joint votre facture %s relative à votre abonnement RelaisDesk (Plan %s - %.2f €).

Détails de la facture :
--------------------------------------------------
Numéro de Facture   : %s
Prestation          : Abonnement RelaisDesk - Plan %s
Montant Net à Payer : %.2f € (TVA non applicable, art. 293 B du CGI)
Statut              : Payée
Pièce jointe        : %s.pdf
--------------------------------------------------

Émetteur :
Julien BELLOT — Entrepreneur individuel (EI)
Noms commerciaux : Informatique A Domicile 03 / RelaisDesk
SIRET : 94074710800014 — Code APE : 9511Z
2 Chemin de Lonzais, 03170 Bizeneuille, France
Téléphone : 06 62 85 59 30

Pour toute question concernant votre facturation : contact@relaisdesk.fr

L'équipe RelaisDesk
https://relaisdesk.fr
`, invoiceNumber, planName, amount, invoiceNumber, planName, amount, invoiceNumber)

	attachmentName := fmt.Sprintf("%s.pdf", invoiceNumber)
	return m.sendRawEmail(toEmail, subject, bodyText, attachmentName, pdfBytes)
}

// SendInvoiceEmail sends simple text invoice notification (fallback if PDF bytes not loaded).
func (m *Mailer) SendInvoiceEmail(toEmail, invoiceNumber, planName, downloadURL string, amount float64) error {
	subject := fmt.Sprintf("Votre facture RelaisDesk (%s)", invoiceNumber)

	bodyText := fmt.Sprintf(`Bonjour,

Votre facture %s relative à votre abonnement RelaisDesk (Plan %s - %.2f €) est disponible.

Détails de la facture :
--------------------------------------------------
Numéro de Facture   : %s
Prestation          : Abonnement RelaisDesk - Plan %s
Montant Net à Payer : %.2f € (TVA non applicable, art. 293 B du CGI)
Statut              : Payée
--------------------------------------------------

Émetteur :
Julien BELLOT — Entrepreneur individuel (EI)
Noms commerciaux : Informatique A Domicile 03 / RelaisDesk
SIRET : 94074710800014 — Code APE : 9511Z
2 Chemin de Lonzais, 03170 Bizeneuille, France
Téléphone : 06 62 85 59 30

Pour toute question : contact@relaisdesk.fr

L'équipe RelaisDesk
https://relaisdesk.fr
`, invoiceNumber, planName, amount, invoiceNumber, planName, amount)

	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

// SendWithdrawalAcknowledgementEmail confirms receipt on a durable medium.
func (m *Mailer) SendWithdrawalAcknowledgementEmail(toEmail, requestID, orderID, requestedAt string) error {
	subject := fmt.Sprintf("Accusé de réception de votre rétractation RelaisDesk (%s)", requestID)
	bodyText := fmt.Sprintf(`Bonjour,

Votre notification de rétractation a été reçue et horodatée.

Référence de la demande : %s
Commande concernée       : %s
Date de réception (UTC)  : %s

Nous examinerons la demande et vous informerons de ses suites. Conservez ce message comme preuve de réception.

Julien BELLOT — Entrepreneur individuel (EI)
Informatique A Domicile 03 / RelaisDesk
SIRET : 94074710800014
2 Chemin de Lonzais, 03170 Bizeneuille, France
Téléphone : 06 62 85 59 30
contact@relaisdesk.fr
`, requestID, orderID, requestedAt)
	return m.sendRawEmail(toEmail, subject, bodyText, "", nil)
}

// SendWithdrawalNotificationEmail alerts the seller of a recorded notice.
func (m *Mailer) SendWithdrawalNotificationEmail(requestID, orderID, customerEmail, customerName, requestedAt string) error {
	target := m.extractEmail(m.From)
	if target == "" {
		return fmt.Errorf("adresse de notification de rétractation indisponible")
	}
	subject := fmt.Sprintf("Rétractation RelaisDesk à traiter (%s)", requestID)
	bodyText := fmt.Sprintf(`Une notification de rétractation a été enregistrée.

Référence demande : %s
Commande           : %s
Client             : %s
E-mail             : %s
Réception (UTC)    : %s

Consultez l'API d'administration et traitez la demande dans les délais applicables.
`, requestID, orderID, customerName, customerEmail, requestedAt)
	return m.sendRawEmail(target, subject, bodyText, "", nil)
}

func (m *Mailer) getPortDescription() string {
	if m.Port == 465 {
		return "TLS direct / Implicit"
	}
	return "STARTTLS / Explicit"
}

// SendContactEmail forwards a contact message from the public website to the administrator.
func (m *Mailer) SendContactEmail(senderName, senderEmail, subject, messageContent string) error {
	adminEmail := m.From
	mailSubject := fmt.Sprintf("[Contact RelaisDesk] %s", subject)
	if strings.TrimSpace(subject) == "" {
		mailSubject = fmt.Sprintf("[Contact RelaisDesk] Message de %s", senderName)
	}

	bodyText := fmt.Sprintf(`Bonjour,

Nouveau message reçu depuis le formulaire de contact du site RelaisDesk :

Nom      : %s
Email    : %s
Date     : %s

Objet    : %s
--------------------------------------------------
%s
--------------------------------------------------

Pour répondre à ce message, vous pouvez répondre directement à cet email (%s).
`, senderName, senderEmail, time.Now().Format("02/01/2006 à 15:04:05"), subject, messageContent, senderEmail)

	return m.sendRawEmailWithReplyTo(adminEmail, senderEmail, mailSubject, bodyText, "", nil)
}

// sendRawEmail performs connection, authentication and transmission with detailed error messages.
func (m *Mailer) sendRawEmail(toEmail, subject, bodyText, attachmentName string, attachmentBytes []byte) error {
	return m.sendRawEmailWithReplyTo(toEmail, "", subject, bodyText, attachmentName, attachmentBytes)
}

func (m *Mailer) sendRawEmailWithReplyTo(toEmail, replyTo, subject, bodyText, attachmentName string, attachmentBytes []byte) error {
	if !m.IsConfigured() {
		return fmt.Errorf("SMTP non configuré: message non remis")
	}

	toAddress, err := mail.ParseAddress(strings.TrimSpace(toEmail))
	if err != nil || toAddress.Address == "" {
		return fmt.Errorf("adresse destinataire invalide")
	}
	fromAddress, err := mail.ParseAddress(strings.TrimSpace(m.From))
	if err != nil || fromAddress.Address == "" {
		return fmt.Errorf("adresse expéditeur invalide")
	}
	fromHeader := fromAddress.String()
	cleanTo := toAddress.Address
	cleanSubject := strings.ReplaceAll(strings.ReplaceAll(subject, "\r", ""), "\n", "")
	cleanFrom := strings.ReplaceAll(strings.ReplaceAll(fromHeader, "\r", ""), "\n", "")
	encodedSubject := mime.QEncoding.Encode("UTF-8", cleanSubject)
	messageRandom := make([]byte, 12)
	if _, err := rand.Read(messageRandom); err != nil {
		return fmt.Errorf("génération Message-ID: %w", err)
	}
	fromParts := strings.Split(fromAddress.Address, "@")
	messageDomain := "localhost"
	if len(fromParts) == 2 && fromParts[1] != "" {
		messageDomain = fromParts[1]
	}
	actualReplyTo := fromAddress.Address
	if cleanReply := strings.TrimSpace(replyTo); cleanReply != "" {
		if parsedReply, err := mail.ParseAddress(cleanReply); err == nil && parsedReply.Address != "" {
			actualReplyTo = parsedReply.Address
		}
	}
	commonHeaders := fmt.Sprintf("From: %s\r\nTo: %s\r\nReply-To: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nSubject: %s\r\nMIME-Version: 1.0\r\nAuto-Submitted: auto-generated\r\n",
		cleanFrom, cleanTo, actualReplyTo, time.Now().Format(time.RFC1123Z), hex.EncodeToString(messageRandom), messageDomain, encodedSubject)

	var msgBuffer bytes.Buffer
	boundary := fmt.Sprintf("relaisdesk_boundary_%d", time.Now().UnixNano())

	if len(attachmentBytes) > 0 && attachmentName != "" {
		// Multipart/mixed message with attachment
		msgBuffer.WriteString(commonHeaders)
		msgBuffer.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

		// Part 1: Text body
		msgBuffer.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msgBuffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		msgBuffer.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		msgBuffer.WriteString(bodyText)
		msgBuffer.WriteString("\r\n\r\n")

		// Part 2: attachment
		safeAttachmentName := sanitizeAttachmentName(attachmentName)
		attachmentType := attachmentContentType(safeAttachmentName)
		msgBuffer.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msgBuffer.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", attachmentType, safeAttachmentName))
		msgBuffer.WriteString("Content-Transfer-Encoding: base64\r\n")
		msgBuffer.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", safeAttachmentName))

		b64Data := base64.StdEncoding.EncodeToString(attachmentBytes)
		// Write in 76 character lines per RFC 2045
		for i := 0; i < len(b64Data); i += 76 {
			end := i + 76
			if end > len(b64Data) {
				end = len(b64Data)
			}
			msgBuffer.WriteString(b64Data[i:end] + "\r\n")
		}
		msgBuffer.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// Simple text message
		msgBuffer.WriteString(commonHeaders)
		msgBuffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		msgBuffer.WriteString(bodyText)
	}

	rawBytes := msgBuffer.Bytes()
	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))

	// Mode 1 : Port 465 -> Direct TLS (Implicit)
	if m.Port == 465 {
		tlsConfig := &tls.Config{
			ServerName: m.Host,
			MinVersion: tls.VersionTLS12,
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		tlsConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("erreur connexion TLS direct sur %s: %w", addr, err)
		}
		defer tlsConn.Close()
		_ = tlsConn.SetDeadline(time.Now().Add(30 * time.Second))

		client, err := smtp.NewClient(tlsConn, m.Host)
		if err != nil {
			return fmt.Errorf("erreur initialisation client SMTP (port 465): %w", err)
		}
		defer client.Quit()

		if m.User != "" && m.Password != "" {
			auth := smtp.PlainAuth("", m.User, m.Password, m.Host)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("authentification SMTP rejetée sur %s (vérifiez SMTP_USER et SMTP_PASS) : %w", addr, err)
			}
		}

		// Extract raw email address from From header
		senderEmail := fromAddress.Address
		if err := client.Mail(senderEmail); err != nil {
			return fmt.Errorf("erreur expéditeur (MAIL FROM: %s) : %w", senderEmail, err)
		}
		if err := client.Rcpt(cleanTo); err != nil {
			return fmt.Errorf("erreur destinataire (RCPT TO: %s) : %w", cleanTo, err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("erreur initialisation données email (DATA) : %w", err)
		}
		if _, err := w.Write(rawBytes); err != nil {
			return fmt.Errorf("erreur écriture corps du message : %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("erreur finalisation envoi : %w", err)
		}

		log.Printf("[MAILER] Email envoyé avec succès à %s (Port 465 TLS)", cleanTo)
		return nil
	}

	// Mode 2 : Port 587 (et autres) -> STARTTLS (Explicit)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("erreur connexion TCP sur %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return fmt.Errorf("erreur initialisation client SMTP sur %s: %w", addr, err)
	}
	defer client.Quit()

	if err := client.Hello("relaisdesk.fr"); err != nil {
		return fmt.Errorf("erreur EHLO SMTP sur %s: %w", addr, err)
	}

	// Port 587 must never fall back to clear text, even without authentication.
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("le serveur SMTP %s ne propose pas STARTTLS", addr)
	}
	tlsConfig := &tls.Config{
		ServerName: m.Host,
		MinVersion: tls.VersionTLS12,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("erreur négociation STARTTLS sur %s: %w", addr, err)
	}

	if m.User != "" && m.Password != "" {
		auth := smtp.PlainAuth("", m.User, m.Password, m.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentification SMTP rejetée sur %s (vérifiez SMTP_USER et SMTP_PASS) : %w", addr, err)
		}
	}

	senderEmail := fromAddress.Address
	if err := client.Mail(senderEmail); err != nil {
		return fmt.Errorf("erreur expéditeur (MAIL FROM: %s) : %w", senderEmail, err)
	}
	if err := client.Rcpt(cleanTo); err != nil {
		return fmt.Errorf("erreur destinataire (RCPT TO: %s) : %w", cleanTo, err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("erreur initialisation données email (DATA) : %w", err)
	}
	if _, err := w.Write(rawBytes); err != nil {
		return fmt.Errorf("erreur écriture corps du message : %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("erreur finalisation envoi : %w", err)
	}

	log.Printf("[MAILER] Email envoyé avec succès à %s (Port %d STARTTLS)", cleanTo, m.Port)
	return nil
}

func sanitizeAttachmentName(name string) string {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(name), "\r", ""), "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\"", "")
	if cleaned == "" {
		return "document.txt"
	}
	return cleaned
}

func attachmentContentType(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".pdf") {
		return "application/pdf"
	}
	return "text/plain; charset=UTF-8"
}

func (m *Mailer) extractEmail(from string) string {
	start := strings.Index(from, "<")
	end := strings.Index(from, ">")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(from[start+1 : end])
	}
	return strings.TrimSpace(from)
}

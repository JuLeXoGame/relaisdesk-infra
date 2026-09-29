package mailer

import (
	"bytes"
	"strings"
	"testing"
)

func TestFleetTermsPreserveHistoricalAttachments(t *testing.T) {
	for _, version := range []string{"2026-09-09", "2026-09-09-trial-v2"} {
		_, body, err := termsAttachment(version)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "500 postes enregistrables") || strings.Contains(string(body), "2026-09-10") {
			t.Fatal("historical terms rewritten", version)
		}
	}
	for _, version := range []string{CurrentTermsVersion, CurrentTrialTermsVersion} {
		_, body, err := termsAttachment(version)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "500 postes enregistrables") || !strings.Contains(string(body), "4 500") {
			t.Fatal("new quota terms missing", version)
		}
	}
}

func TestMailerConfiguration(t *testing.T) {
	mUnconfigured := NewMailer("", 0, "", "", "")
	if mUnconfigured.IsConfigured() {
		t.Errorf("Expected IsConfigured to be false for empty mailer")
	}

	mConfigured := NewMailer("mail.informatiqueadomicile03.fr", 587, "contact@relaisdesk.fr", "secret", "RelaisDesk <contact@relaisdesk.fr>")
	if !mConfigured.IsConfigured() {
		t.Errorf("Expected IsConfigured to be true for configured mailer")
	}

	emailExtracted := mConfigured.extractEmail(mConfigured.From)
	if emailExtracted != "contact@relaisdesk.fr" {
		t.Errorf("Expected contact@relaisdesk.fr, got %s", emailExtracted)
	}

	// Mock send (unconfigured) shouldn't fail
	err := mUnconfigured.SendTestEmail("test@example.com")
	if err == nil {
		t.Errorf("Expected error when sending test email on unconfigured mailer")
	}
	if err := mUnconfigured.SendLicenseEmail("client@example.com", "Starter", "MP-TEST", "secret", "01/09/2026", 1); err == nil {
		t.Fatal("unconfigured SMTP silently accepted a transactional message")
	}
}

func TestEmbeddedContractualTermsAreVersionedAndComplete(t *testing.T) {
	content := strings.ToUpper(string(contractualTerms))
	for _, expected := range []string{
		"VERSION CONTRACTUELLE " + CurrentTermsVersion,
		"JULIEN BELLOT",
		"940 747 108 00014",
		"DROIT DE RÉTRACTATION",
		"GARANTIE LÉGALE",
		"GNU AGPLV3",
		"24,90",
		"365 JOURS",
		"ANNEXE DE SOUS-TRAITANCE RGPD",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("conditions contractuelles embarquées incomplètes : %q absent", expected)
		}
	}
}

func TestContractualTermsFollowTheRecordedOrderVersion(t *testing.T) {
	for _, version := range []string{"2026-08-25", "2026-09-07", "2026-09-08", "2026-09-09", "2026-09-10", "2026-09-11", CurrentTermsVersion} {
		name, body, err := termsAttachment(version)
		if err != nil || name != "CGV-RelaisDesk-"+version+".txt" || !strings.Contains(string(body), "Version contractuelle "+version) {
			t.Fatalf("incorrect archive for %s: %s, %v", version, name, err)
		}
	}
	for _, versions := range [][]string{nil, {""}} {
		name, body, err := termsAttachment(versions...)
		if err != nil || name != "" || body != nil {
			t.Fatal("legacy order was assigned unaccepted terms")
		}
	}
	for _, versions := range [][]string{{"2099-01-01"}, {"../secret"}, {CurrentTermsVersion, "2026-08-25"}} {
		if _, _, err := termsAttachment(versions...); err == nil {
			t.Fatal("unarchived or ambiguous version silently accepted")
		}
	}
	if strings.Contains(string(previousContractualTerms), "Version contractuelle "+CurrentTermsVersion) {
		t.Fatal("historical terms overwritten")
	}
}

func TestAttachmentMetadataIsSafeAndTyped(t *testing.T) {
	if got := attachmentContentType("conditions.TXT"); got != "text/plain; charset=UTF-8" {
		t.Fatalf("type texte = %q", got)
	}
	if got := attachmentContentType("facture.PDF"); got != "application/pdf" {
		t.Fatalf("type PDF = %q", got)
	}
	if got := sanitizeAttachmentName("facture\r\nInjected: yes.pdf"); strings.ContainsAny(got, "\r\n\"") {
		t.Fatalf("nom de pièce jointe non nettoyé : %q", got)
	}
}

func TestProPriceChangePreservesHistoricalEmailTerms(t *testing.T) {
	for _, tc := range []struct{ version, price string }{
		{"2026-09-07", "Pro : 129,00 € pour 30 jours ou 1 290,00 € pour 365 jours"},
		{CurrentTermsVersion, "Pro : 110,00 € pour 30 jours ou 1 100,00 € pour 365 jours"},
	} {
		_, content, err := termsAttachment(tc.version)
		if err != nil || !strings.Contains(string(content), tc.price) {
			t.Fatalf("incorrect price archive for %s: %v", tc.version, err)
		}
	}
}

func TestCustomPriceChangePreservesHistoricalEmailTerms(t *testing.T) {
	for _, tc := range []struct{ version, price string }{
		{"2026-09-07", "à partir de 239,00 € pour 30 jours ou 2 390,00 € pour 365 jours"},
		{CurrentTermsVersion, "à partir de 199,00 € pour 30 jours ou 1 990,00 € pour 365 jours"},
	} {
		_, content, err := termsAttachment(tc.version)
		if err != nil || !strings.Contains(string(content), tc.price) {
			t.Fatalf("incorrect custom price archive for %s: %v", tc.version, err)
		}
	}
}

func TestPermanentAccessTermsDoNotRewriteFleetV1(t *testing.T) {
	for _, tc := range []struct {
		version string
		name    string
		want    []byte
	}{
		{"2026-09-10", "CGV-RelaisDesk-2026-09-10.txt", september10ContractualTerms},
		{"2026-09-10-fleet-v1", "CGV-et-ESSAI-RelaisDesk-2026-09-10-fleet-v1.txt",
			append(append([]byte{}, september10ContractualTerms...), september10TrialSupplement...)},
	} {
		name, body, err := termsAttachment(tc.version)
		if err != nil || name != tc.name || !bytes.Equal(body, tc.want) {
			t.Fatalf("historical fleet contract changed for %s: %s, %v", tc.version, name, err)
		}
		if bytes.Contains(body, []byte("autorisation préalable documentée")) {
			t.Fatal("new permanent-access conditions inserted into an existing contract")
		}
	}
	for _, version := range []string{CurrentTermsVersion, CurrentTrialTermsVersion} {
		name, body, err := termsAttachment(version)
		if err != nil || !strings.Contains(name, version) {
			t.Fatalf("current attachment: %s, %v", name, err)
		}
		for _, clause := range []string{"autorisation préalable documentée", "sans validation interactive", "procédure manuelle", "Version contractuelle 2026-09-27", "Linux ou macOS", "ne désinstalle pas", "Engagement de disponibilité (SLA)"} {
			if !bytes.Contains(body, []byte(clause)) {
				t.Fatalf("%s missing clause %q", version, clause)
			}
		}
	}
}

func TestCryptoPaymentTermsDoNotRewritePreviousVersion(t *testing.T) {
	for _, version := range []string{"2026-09-11", "2026-09-11-fleet-v2"} {
		_, body, err := termsAttachment(version)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("Destination Tag")) || bytes.Contains(body, []byte("crypto-actifs")) {
			t.Fatal("new crypto payment conditions inserted into an existing contract", version)
		}
	}
	for _, version := range []string{CurrentTermsVersion, CurrentTrialTermsVersion} {
		_, body, err := termsAttachment(version)
		if err != nil {
			t.Fatal(err)
		}
		for _, clause := range []string{"Crypto-actifs (Bitcoin, XRP)", "Destination Tag", "sans réexpédition de crypto-actifs", "donnent lieu à aucun complément ni retenue"} {
			if !bytes.Contains(body, []byte(clause)) {
				t.Fatalf("%s missing clause %q", version, clause)
			}
		}
	}
}

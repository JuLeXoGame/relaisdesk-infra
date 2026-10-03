package main

// Non-régression : une seule connexion au lancement (technicien) doit
// ouvrir aussi la session client en silence, sans second formulaire.
// Échec silencieux -> saisie manuelle ; 2FA -> formulaire de code direct.

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func resetCustomerSilentState(t *testing.T) {
	t.Helper()
	oldToken, oldEmail, oldID := customerSessionToken, customerSessionEmail, customerSessionCustomerID
	oldCh, oldChEmail, oldChAllowed := customerPendingChallenge, customerPendingEmail, customerPendingEmailAllowed
	oldURL, oldTransport := APIURL, http.DefaultTransport
	customerSessionToken, customerSessionEmail, customerSessionCustomerID = "", "", ""
	customerPendingChallenge, customerPendingEmail, customerPendingEmailAllowed = "", "", false
	APIURL = "https://customer.invalid"
	t.Cleanup(func() {
		customerSessionToken, customerSessionEmail, customerSessionCustomerID = oldToken, oldEmail, oldID
		customerPendingChallenge, customerPendingEmail, customerPendingEmailAllowed = oldCh, oldChEmail, oldChAllowed
		APIURL, http.DefaultTransport = oldURL, oldTransport
	})
}

func mockCustomerAuth(t *testing.T, body string, status int, check func(*http.Request)) {
	t.Helper()
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if check != nil {
			check(r)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
}

func TestSilentCustomerLoginStoresSession(t *testing.T) {
	resetCustomerSilentState(t)
	var sawEmail, sawPassword bool
	mockCustomerAuth(t, `{"token":"tok-silent","customer_id":"CUS-9","email":"a@b.c"}`, 200, func(r *http.Request) {
		if r.URL.Path != "/api/v1/customer/login" {
			t.Fatal("mauvais endpoint", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		sawEmail = strings.Contains(string(raw), "a@b.c")
		sawPassword = strings.Contains(string(raw), "motdepasse123")
	})
	openCustomerSessionSilent("a@b.c", "motdepasse123", "dev-1")
	if !customerLoggedIn() || customerSessionToken != "tok-silent" || customerSessionEmail != "a@b.c" {
		t.Fatalf("session silencieuse attendue: token=%q email=%q", customerSessionToken, customerSessionEmail)
	}
	if !sawEmail || !sawPassword {
		t.Fatalf("identifiants technicien non reutilises: email=%v password=%v", sawEmail, sawPassword)
	}
}

func TestSilentCustomerLoginStashes2FAChallenge(t *testing.T) {
	resetCustomerSilentState(t)
	mockCustomerAuth(t, `{"requires_2fa":true,"challenge_token":"ch-silent","customer_id":"CUS-9","email":"a@b.c"}`, 200, nil)
	openCustomerSessionSilent("a@b.c", "motdepasse123", "")
	if customerLoggedIn() {
		t.Fatal("pas de session sans code 2FA")
	}
	if customerPendingChallenge != "ch-silent" || customerPendingEmail != "a@b.c" || !customerPendingEmailAllowed {
		t.Fatalf("challenge en attente attendu: %+v", customerPendingChallenge)
	}
}

func TestSilentCustomerLoginFailureStaysSilent(t *testing.T) {
	resetCustomerSilentState(t)
	mockCustomerAuth(t, `{"error":"Identifiants incorrects"}`, 401, nil)
	openCustomerSessionSilent("a@b.c", "mauvais", "")
	if customerLoggedIn() || customerPendingChallenge != "" {
		t.Fatal("echec silencieux attendu (saisie manuelle ensuite)")
	}
	openCustomerSessionSilent("", "", "")
	openCustomerSessionSilentGoogle("  ")
}

func TestSilentCustomerLoginGoogle(t *testing.T) {
	resetCustomerSilentState(t)
	var sawCredential bool
	mockCustomerAuth(t, `{"token":"tok-g","customer_id":"CUS-9","email":"a@b.c"}`, 200, func(r *http.Request) {
		if r.URL.Path != "/api/v1/customer/login/google" {
			t.Fatal("mauvais endpoint", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		sawCredential = strings.Contains(string(raw), "cred-abc")
	})
	openCustomerSessionSilentGoogle("cred-abc")
	if !customerLoggedIn() || customerSessionToken != "tok-g" {
		t.Fatalf("session Google silencieuse attendue: %q", customerSessionToken)
	}
	if !sawCredential {
		t.Fatal("jeton Google non reutilise")
	}
}

func TestStoreCustomerSessionClearsPending(t *testing.T) {
	resetCustomerSilentState(t)
	customerPendingChallenge, customerPendingEmail, customerPendingEmailAllowed = "ch-1", "a@b.c", true
	storeCustomerSession(&customerLoginResult{Token: "tok-1", Email: "a@b.c", CustomerID: "CUS-1"})
	if customerPendingChallenge != "" || customerPendingEmail != "" || customerPendingEmailAllowed {
		t.Fatal("challenge en attente non efface apres connexion")
	}
	stashCustomerChallenge(&customerLoginResult{ChallengeToken: "ch-2", Email: "a@b.c"})
	if !customerPendingEmailAllowed {
		t.Fatal("code e-mail autorise par defaut quand le serveur ne precise pas")
	}
	explicitFalse := false
	stashCustomerChallenge(&customerLoginResult{ChallengeToken: "ch-3", Email: "a@b.c", EmailCodeAllowed: &explicitFalse})
	if customerPendingEmailAllowed {
		t.Fatal("interdiction serveur du code e-mail ignoree")
	}
}

func TestCustomerGatePanelShows2FADirectly(t *testing.T) {
	resetCustomerSilentState(t)
	noop := func(int) {}
	sentinel := widget.NewLabel("contenu-client")
	builder := func(int, func(int)) fyne.CanvasObject { return sentinel }

	// Sans session ni challenge : formulaire complet (email + mot de passe).
	if n := countPanelEntries(customerGatePanel(PanelOverview, noop, builder)); n != 2 {
		t.Fatalf("formulaire complet attendu (2 champs), obtenu %d", n)
	}
	// Challenge en attente : formulaire de code direct (1 champ).
	customerPendingChallenge, customerPendingEmail = "ch-9", "a@b.c"
	if n := countPanelEntries(customerGatePanel(PanelOverview, noop, builder)); n != 1 {
		t.Fatalf("formulaire de code attendu (1 champ), obtenu %d", n)
	}
	// Session ouverte : contenu du builder.
	storeCustomerSession(&customerLoginResult{Token: "tok-1", Email: "a@b.c", CustomerID: "CUS-1"})
	if got := customerGatePanel(PanelOverview, noop, builder); got != sentinel {
		t.Fatal("contenu client attendu quand la session est ouverte")
	}
}

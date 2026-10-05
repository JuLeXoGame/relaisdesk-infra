package main

// Régression : les panneaux compte client affichaient une vignette de 32px
// ("Connexion — Espace client" seule, inutilisable) au lieu du formulaire.
// Cause : un VScroll enveloppé dans container.NewCenter — la hauteur min
// d'un scroll vertical vaut 32px (Fyne), Center réduit donc l'enfant à sa
// MinSize. Ces tests montent les panneaux dans une fenêtre headless et
// exigent un scroll à taille réelle.

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func collectPanelScrolls(obj fyne.CanvasObject, out *[]*container.Scroll) {
	switch o := obj.(type) {
	case *container.Scroll:
		*out = append(*out, o)
		if o.Content != nil {
			collectPanelScrolls(o.Content, out)
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			collectPanelScrolls(child, out)
		}
	}
}

func countPanelEntries(obj fyne.CanvasObject) int {
	n := 0
	switch o := obj.(type) {
	case *widget.Entry:
		n++
	case *container.Scroll:
		if o.Content != nil {
			n += countPanelEntries(o.Content)
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			n += countPanelEntries(child)
		}
	}
	return n
}

func assertUsablePanelScroll(t *testing.T, name string, panel fyne.CanvasObject) {
	t.Helper()
	a := test.NewApp()
	defer a.Quit()
	SetLang(LangFR)
	defer SetLang(LangFR)
	w := a.NewWindow(name)
	defer w.Close()
	w.SetContent(panel)
	w.Resize(fyne.NewSize(1000, 700))
	w.Content().Refresh()
	var scrolls []*container.Scroll
	collectPanelScrolls(w.Content(), &scrolls)
	if len(scrolls) == 0 {
		t.Fatalf("%s: aucun scroll trouve", name)
	}
	for _, s := range scrolls {
		if h := s.Size().Height; h < 400 {
			t.Errorf("%s: scroll affiche sur %.0fpx de haut, formulaire ecrase (attendu > 400)", name, h)
		}
	}
}

func TestCustomerLoginPanelShowsFullForm(t *testing.T) {
	noop := func(int) {}
	assertUsablePanelScroll(t, "login", customerLoginPanel(PanelOverview, noop, ""))
	if n := countPanelEntries(customerLoginPanel(PanelOverview, noop, "")); n < 2 {
		t.Errorf("panneau connexion: %d champs texte, attendu >= 2 (email + mot de passe)", n)
	}
}

func TestCustomer2FAPanelShowsFullForm(t *testing.T) {
	noop := func(int) {}
	assertUsablePanelScroll(t, "2fa", customer2FAPanel("challenge-test", "test@example.com", true, PanelOverview, noop))
}

func TestSidebarSplitOffset(t *testing.T) {
	if got := sidebarSplitOffset(false, 1600, 300); got != 0.24 {
		t.Fatalf("deplie: ratio %v, attendu 0.24", got)
	}
	if got := sidebarSplitOffset(true, 1600, 56); got < 0.02 || got > 0.12 {
		t.Fatalf("replie: ratio %v hors bornes [0.02, 0.12]", got)
	}
	if got, want := sidebarSplitOffset(true, 1600, 56), 64.0/1600.0; got-want > 1e-6 || want-got > 1e-6 {
		t.Fatalf("replie: ratio %v, attendu %v", got, want)
	}
	if got := sidebarSplitOffset(true, 0, 0); got != 0.05 {
		t.Fatalf("replie sans dimensions: ratio %v, attendu 0.05", got)
	}
}

func TestToggleSidebarCollapsedPersists(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Preferences().SetBool("sidebar_collapsed", false)
	if got := toggleSidebarCollapsed(false); !got {
		t.Fatal("bascule vers replie attendue")
	}
	if !a.Preferences().BoolWithFallback("sidebar_collapsed", false) {
		t.Fatal("etat replie non persiste")
	}
	if got := toggleSidebarCollapsed(true); got {
		t.Fatal("bascule vers deplie attendue")
	}
	if a.Preferences().BoolWithFallback("sidebar_collapsed", true) {
		t.Fatal("etat deplie non persiste")
	}
}

func TestSidebarNavLabelsFollowCollapsed(t *testing.T) {
	noop := func(int) {}
	expanded, ok := buildSidebarNav(PanelCodes, false, false, noop, nil).(*fyne.Container)
	if !ok || len(expanded.Objects) == 0 {
		t.Fatal("navigation depliee vide")
	}
	for _, o := range expanded.Objects {
		if b, ok := o.(*widget.Button); ok && b.Text == "" {
			t.Fatal("libelle manquant en mode deplie")
		}
	}
	collapsed, ok := buildSidebarNav(PanelCodes, true, false, noop, nil).(*fyne.Container)
	if !ok || len(collapsed.Objects) != len(expanded.Objects) {
		t.Fatal("navigation repliee incoherente")
	}
	for _, o := range collapsed.Objects {
		b, ok := o.(*widget.Button)
		if !ok {
			continue
		}
		if b.Text != "" {
			t.Fatalf("libelle %q visible en mode replie (icones seules attendues)", b.Text)
		}
		if b.Icon == nil {
			t.Fatal("icone manquante en mode replie")
		}
	}
}

func TestErrorURLsExtraction(t *testing.T) {
	stripeMsg := "Stripe : You must complete your platform profile to use Connect and create live connected accounts. Visit your dashboard at https://dashboard.stripe.com/connect/accounts/overview to answer the questionnaire."
	urls := errorURLs(stripeMsg)
	if len(urls) != 1 || urls[0] != "https://dashboard.stripe.com/connect/accounts/overview" {
		t.Fatalf("lien Stripe attendu: %v", urls)
	}
	if len(errorURLs("erreur simple sans lien")) != 0 {
		t.Fatal("aucun lien attendu")
	}
	dupes := errorURLs("voir https://a.example/x. puis https://a.example/x")
	if len(dupes) != 1 || dupes[0] != "https://a.example/x" {
		t.Fatalf("ponctuation/dedoublonnage: %v", dupes)
	}
}

func collectPanelLinks(obj fyne.CanvasObject, out *[]*widget.Hyperlink) {
	switch o := obj.(type) {
	case *widget.Hyperlink:
		*out = append(*out, o)
	case *container.Scroll:
		if o.Content != nil {
			collectPanelLinks(o.Content, out)
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			collectPanelLinks(child, out)
		}
	}
}

func TestErrorWithLinksContentHasHyperlinks(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	content := errorWithLinksContent("Stripe : complétez votre profil sur https://dashboard.stripe.com/connect/accounts/overview merci.")
	var links []*widget.Hyperlink
	collectPanelLinks(content, &links)
	if len(links) != 1 {
		t.Fatalf("1 lien cliquable attendu, obtenu %d", len(links))
	}
	if links[0].URL == nil || links[0].URL.Host != "dashboard.stripe.com" {
		t.Fatalf("URL lien inattendue: %+v", links[0].URL)
	}
	if links[0].OnTapped == nil {
		t.Fatal("lien sans action (ouverture navigateur manquante)")
	}
	w := a.NewWindow("err")
	defer w.Close()
	showErrorWithLinks(errors.New("avec https://a.example/x lien"), w)
	showErrorWithLinks(errors.New("sans lien"), w)
	showErrorWithLinks(nil, w)
}

// Régression : le dialogue « Dossiers » de l'onglet équipe s'ouvrait écrasé
// (VScroll sans taille min). On reproduit le dimensionnement d'un dialogue
// (fenêtre à la MinSize du contenu) et on exige une liste utilisable.
func TestTeamFoldersDialogContentHasUsableHeight(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	SetLang(LangFR)
	defer SetLang(LangFR)
	member := customerTeamMember{Email: "membre@example.test", FolderIDs: []string{"f1"}}
	folders := []customerFolder{
		{FolderID: "f1", Name: "Compta"},
		{FolderID: "f2", Name: "Technique"},
		{FolderID: "f3", Name: "Direction"},
	}
	content, check, _ := teamFoldersDialogContent(member, folders)
	if len(check.Selected) != 1 {
		t.Fatalf("présélection = %d cases, attendu 1", len(check.Selected))
	}
	w := a.NewWindow("dossiers")
	defer w.Close()
	w.SetContent(content)
	w.Resize(content.MinSize())
	w.Content().Refresh()
	var scrolls []*container.Scroll
	collectPanelScrolls(w.Content(), &scrolls)
	if len(scrolls) != 1 {
		t.Fatalf("attendu 1 scroll, trouvé %d", len(scrolls))
	}
	if h := scrolls[0].Size().Height; h < 150 {
		t.Errorf("scroll affiche sur %.0fpx de haut, liste écrasee (attendu >= 150)", h)
	}
}

func TestFolderRootLabelHasNoEmojiIcon(t *testing.T) {
	defer SetLang(LangFR)
	SetLang(LangFR)
	if label := T("folder_root"); strings.Contains(label, "📁") {
		t.Errorf("folder_root FR %q contient l'emoji dossier orange", label)
	}
	SetLang(LangEN)
	if label := T("folder_root"); strings.Contains(label, "📁") {
		t.Errorf("folder_root EN %q contient l'emoji dossier orange", label)
	}
}

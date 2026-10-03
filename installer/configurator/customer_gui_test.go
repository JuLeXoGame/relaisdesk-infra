package main

// Régression : les panneaux compte client affichaient une vignette de 32px
// ("Connexion — Espace client" seule, inutilisable) au lieu du formulaire.
// Cause : un VScroll enveloppé dans container.NewCenter — la hauteur min
// d'un scroll vertical vaut 32px (Fyne), Center réduit donc l'enfant à sa
// MinSize. Ces tests montent les panneaux dans une fenêtre headless et
// exigent un scroll à taille réelle.

import (
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

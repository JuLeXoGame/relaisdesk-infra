package main

// Régression : sur Windows, la molette déplaçait le contenu par à-coups
// (25 px par cran d'un coup sec, x125 en rotation rapide). Les scrolls
// adoucis doivent accumuler puis rejouer les deltas en douceur, borner
// les rafales, et converger vers le repos.

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// stubSmoothAsync neutralise la replanification asynchrone : le test
// pilote step() à la main, de façon déterministe (aucune goroutine).
func stubSmoothAsync(t *testing.T) {
	t.Helper()
	orig := smoothScrollAfter
	smoothScrollAfter = func(time.Duration, func()) *time.Timer { return nil }
	t.Cleanup(func() { smoothScrollAfter = orig })
}

func TestClampScrollDelta(t *testing.T) {
	if got := clampScrollDelta(25); got != 25 {
		t.Fatalf("cran normal modifié : %v", got)
	}
	if got := clampScrollDelta(-3.5); got != -3.5 {
		t.Fatalf("petit delta tactile modifié : %v", got)
	}
	// Borne par évènement : ~un écran, pas 120 px (un élan franc écrasé à
	// 120 px donne un bond suivi d'un arrêt net — le « ça s'arrête » signalé).
	if got := clampScrollDelta(3000); got != 720 {
		t.Fatalf("rafale bornée à %v, attendu 720", got)
	}
	if got := clampScrollDelta(-5000); got != -720 {
		t.Fatalf("rafale négative bornée à %v, attendu -720", got)
	}
}

func TestSmoothStepDeltaConverges(t *testing.T) {
	if got := smoothStepDelta(0); got != 0 {
		t.Fatalf("repos non nul : %v", got)
	}
	// Convergence : la somme des étapes doit solder exactement le restant.
	remaining := float32(100)
	var acc float32
	for i := 0; i < 1000; i++ {
		d := smoothStepDelta(remaining)
		if d == 0 {
			t.Fatalf("étape nulle avec restant %v (asymptote infinie)", remaining)
		}
		acc += d
		remaining -= d
		if remaining == 0 {
			break
		}
	}
	if remaining != 0 {
		t.Fatalf("non convergé après 1000 étapes, restant %v", remaining)
	}
	if d := acc - 100; d > 0.01 || d < -0.01 {
		t.Fatalf("distance totale %v, attendu 100", acc)
	}
	// Progressivité : la première étape ne rejoue qu'une fraction.
	if d := smoothStepDelta(100); d >= 100 || d <= 0 {
		t.Fatalf("première étape %v : pas de fractionnement", d)
	}
}

func driveSmoothToRest(t *testing.T, s *smoothScroll) {
	t.Helper()
	for i := 0; i < 1000 && s.animating; i++ {
		s.step()
	}
	if s.animating {
		t.Fatal("animation non terminée après 1000 étapes")
	}
}

func TestSmoothVScrollWheelEndToEnd(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	rows := container.NewVBox()
	for i := 0; i < 50; i++ {
		rows.Add(widget.NewLabel("ligne"))
	}
	s := NewSmoothVScroll(rows)
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 200))

	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if s.targetY != -100 {
		t.Fatalf("delta non accumulé : targetY=%v", s.targetY)
	}
	// Pas de saut instantané : l'offset ne bouge qu'en rejouant les étapes.
	if s.Scroll.Offset.Y != 0 {
		t.Fatalf("saut instantané %.0fpx (comportement saccadé)", s.Scroll.Offset.Y)
	}
	driveSmoothToRest(t, s)
	if s.Scroll.Offset.Y <= 0 {
		t.Fatalf("offset final %.1f : le contenu n'a pas défilé", s.Scroll.Offset.Y)
	}
	if s.targetX != 0 || s.targetY != 0 {
		t.Fatalf("restant non soldé : (%v, %v)", s.targetX, s.targetY)
	}
}

func TestSmoothHScrollWheelDrivesHorizontal(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	row := container.NewHBox()
	for i := 0; i < 50; i++ {
		row.Add(widget.NewLabel("colonne"))
	}
	s := NewSmoothHScroll(row)
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 100))

	// Molette verticale sur scroll horizontal : Fyne fait défiler à
	// l'horizontale quand le contenu tient en hauteur — le lissage suit.
	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if s.targetY != -100 {
		t.Fatalf("delta non accumulé : targetY=%v", s.targetY)
	}
	driveSmoothToRest(t, s)
	if s.Scroll.Offset.X <= 0 {
		t.Fatalf("offset X final %.1f : pas de défilement horizontal", s.Scroll.Offset.X)
	}
	if s.Scroll.Offset.Y != 0 {
		t.Fatalf("offset Y final %.1f : dérive verticale inattendue", s.Scroll.Offset.Y)
	}
}

func TestSmoothScrollStopsAtEdge(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	// Contenu plus petit que la vue : aucun défilement possible,
	// l'animation doit s'arrêter au lieu de tourner dans le vide.
	s := NewSmoothVScroll(widget.NewLabel("court"))
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 200))

	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -500)})
	driveSmoothToRest(t, s)
	if s.Scroll.Offset.Y != 0 {
		t.Fatalf("offset %.1f sur contenu non défilable", s.Scroll.Offset.Y)
	}
}

func TestSmoothScrollFastFlickTravels(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	rows := container.NewVBox()
	for i := 0; i < 200; i++ {
		rows.Add(widget.NewLabel("ligne"))
	}
	s := NewSmoothVScroll(rows)
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 200))

	// Rafale d'accélération Windows (x125, ~31 000 px demandés) : elle doit
	// parcourir ~720 px en continu, pas 120 px suivis d'un arrêt net.
	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -31250)})
	if s.targetY != -720 {
		t.Fatalf("rafale accumulée %v, attendu -720", s.targetY)
	}
	driveSmoothToRest(t, s)
	if s.Scroll.Offset.Y < 719 || s.Scroll.Offset.Y > 721 {
		t.Fatalf("offset final %.1f : le geste n'a pas parcouru ~720 px", s.Scroll.Offset.Y)
	}
}

func TestSmoothScrollPendingCapped(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	rows := container.NewVBox()
	for i := 0; i < 200; i++ {
		rows.Add(widget.NewLabel("ligne"))
	}
	s := NewSmoothVScroll(rows)
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 200))

	// Élan soutenu : le cumul est plafonné (~5 écrans), sans emballement.
	for i := 0; i < 10; i++ {
		s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -1000)})
	}
	if s.targetY != -3600 {
		t.Fatalf("cumul %v, attendu -3600", s.targetY)
	}
	driveSmoothToRest(t, s)
	if s.animating || s.targetX != 0 || s.targetY != 0 {
		t.Fatalf("animation non soldée : targets=(%v, %v)", s.targetX, s.targetY)
	}
}

func TestSmoothScrollToTopCancelsPending(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	rows := container.NewVBox()
	for i := 0; i < 50; i++ {
		rows.Add(widget.NewLabel("ligne"))
	}
	s := NewSmoothVScroll(rows)
	w := a.NewWindow("scroll")
	defer w.Close()
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 200))

	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -200)})
	s.ScrollToTop()
	driveSmoothToRest(t, s)
	if s.Scroll.Offset.Y != 0 {
		t.Fatalf("offset %.1f après ScrollToTop : le restant a reflué", s.Scroll.Offset.Y)
	}
}

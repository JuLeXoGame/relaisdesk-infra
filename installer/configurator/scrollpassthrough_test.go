package main

// Régression : sur l'onglet « Équipe & droits », la molette ne faisait plus
// défiler la page quand le curseur survolait un champ texte ou le mini
// explorateur de dossiers. Fyne route chaque cran au Scrollable le plus
// profond sous le curseur, sans remonter au parent : le scroll imbriqué
// (ou le scroll interne d'un Entry) avalait l'évènement même sans pouvoir
// défiler. Les scrolls imbriqués doivent donc faire remonter au scroll de
// page les crans qu'ils ne peuvent pas honorer, et les champs monolignes
// masquer leur scroll interne Fyne tant que leur texte tient dans le champ.

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// showSized affiche un contenu dans une fenêtre de test à la taille voulue
// (les scrolls ont besoin d'une mise en page réelle : tailles allouées).
func showSized(a fyne.App, content fyne.CanvasObject, w, h float32) fyne.Window {
	win := a.NewWindow("bulle")
	win.SetContent(content)
	win.Resize(fyne.NewSize(w, h))
	return win
}

func tallBox(lines int) *fyne.Container {
	rows := container.NewVBox()
	for i := 0; i < lines; i++ {
		rows.Add(widget.NewLabel("ligne"))
	}
	return rows
}

func TestSmoothScrollBubblesWhenContentFits(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	// Inner au contenu court (tient dans sa vue), page haute (défilable).
	inner := NewSmoothVScroll(widget.NewLabel("court"))
	inner.SetMinSize(fyne.NewSize(200, 100))
	page := NewSmoothVScroll(container.NewVBox(inner, tallBox(50)))
	adoptNestedScrolls(page, page.Scroll.Content)
	w := showSized(a, page, 300, 200)
	defer w.Close()

	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if inner.targetY != 0 {
		t.Fatalf("inner a consommé le cran (targetY=%v) alors que son contenu tient", inner.targetY)
	}
	if page.targetY != -100 {
		t.Fatalf("cran non remonté à la page : targetY=%v, attendu -100", page.targetY)
	}
	driveSmoothToRest(t, page)
	if page.Scroll.Offset.Y <= 0 {
		t.Fatalf("offset page %.1f : la page n'a pas défilé", page.Scroll.Offset.Y)
	}
}

func TestSmoothScrollBubblesAtBoundary(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	inner := NewSmoothVScroll(tallBox(50))
	inner.SetMinSize(fyne.NewSize(200, 100))
	page := NewSmoothVScroll(container.NewVBox(inner, tallBox(50)))
	adoptNestedScrolls(page, page.Scroll.Content)
	w := showSized(a, page, 300, 200)
	defer w.Close()

	// En bas de l'inner, cran vers le bas : ça remonte à la page.
	maxY := inner.Scroll.Content.MinSize().Height - inner.Scroll.Size().Height
	for i := 0; i < 10 && inner.Scroll.Offset.Y < maxY-1; i++ {
		inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -2000)})
		driveSmoothToRest(t, inner)
	}
	if inner.Scroll.Offset.Y < maxY-1 {
		t.Fatalf("préparation : inner à %.1f, butée %.1f", inner.Scroll.Offset.Y, maxY)
	}
	page.ScrollToTop() // la préparation a pu faire déborder un cran vers la page
	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if inner.targetY != 0 || page.targetY != -100 {
		t.Fatalf("butée basse : inner=%v page=%v, attendu inner=0 page=-100", inner.targetY, page.targetY)
	}
	driveSmoothToRest(t, page)

	// En haut de l'inner, cran vers le haut : ça remonte aussi.
	inner.ScrollToTop()
	page.targetY = 0
	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, 100)})
	if inner.targetY != 0 || page.targetY != 100 {
		t.Fatalf("butée haute : inner=%v page=%v, attendu inner=0 page=100", inner.targetY, page.targetY)
	}
	driveSmoothToRest(t, page)

	// Au milieu de l'inner : l'inner consomme, la page ne bouge pas.
	inner.Scroll.ScrollToOffset(fyne.NewPos(0, maxY/2))
	page.targetY = 0
	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -50)})
	if inner.targetY != -50 {
		t.Fatalf("milieu : inner a lâché le cran (targetY=%v)", inner.targetY)
	}
	if page.targetY != 0 {
		t.Fatalf("milieu : page a reçu %v, attendu 0", page.targetY)
	}
}

func TestSmoothScrollSnapsStaleOffsetWhenContentShrinks(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	inner := NewSmoothVScroll(tallBox(50))
	inner.SetMinSize(fyne.NewSize(200, 100))
	page := NewSmoothVScroll(container.NewVBox(inner, tallBox(50)))
	adoptNestedScrolls(page, page.Scroll.Content)
	w := showSized(a, page, 300, 200)
	defer w.Close()

	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -5000)})
	driveSmoothToRest(t, inner)
	if inner.Scroll.Offset.Y <= 0 {
		t.Fatal("préparation : inner aurait dû défiler")
	}
	// Le contenu rétrécit (liste filtrée, sans Refresh du scroll) : l'offset
	// hérité pointe dans le vide. Le prochain cran doit le recadrer dans
	// l'intervalle valide (parité Fyne : updateOffset rabat) au lieu
	// d'afficher du vide, puis remonter à la page.
	inner.Scroll.Content = widget.NewLabel("court")
	inner.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if inner.Scroll.Offset.Y != 0 {
		t.Fatalf("offset hérité %.1f : contenu décalé dans le vide", inner.Scroll.Offset.Y)
	}
	if page.targetY != -100 {
		t.Fatalf("cran non remonté après rétrécissement : page=%v", page.targetY)
	}
}

func TestAdoptNestedScrolls(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	inner1 := NewSmoothVScroll(widget.NewLabel("a"))
	inner2 := NewSmoothVScroll(widget.NewLabel("b"))
	card := createCardBox(container.NewVBox(inner1), color.White)
	outer := NewSmoothVScroll(container.NewVBox(card, inner2))

	adoptNestedScrolls(outer, outer.Scroll.Content)
	if inner1.bubbleTo != outer {
		t.Fatal("inner sous carte non rattaché à la page")
	}
	if inner2.bubbleTo != outer {
		t.Fatal("inner direct non rattaché à la page")
	}
	if outer.bubbleTo != nil {
		t.Fatal("la page ne doit pas se rattacher elle-même")
	}

	// Chaînage : un scroll dans le contenu d'un scroll remonte au plus proche.
	deep := NewSmoothVScroll(widget.NewLabel("profond"))
	holder := NewSmoothVScroll(container.NewVBox(deep))
	outer2 := NewSmoothVScroll(holder)
	adoptNestedScrolls(outer2, outer2.Scroll.Content)
	if holder.bubbleTo != outer2 {
		t.Fatal("holder non rattaché à la page")
	}
	if deep.bubbleTo != holder {
		t.Fatal("deep devrait remonter au scroll le plus proche, pas à la page")
	}
}

func TestAdoptNestedScrollsThroughSplit(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	// Forme réelle du panneau Équipe : l'explorateur et la liste vivent
	// dans un HSplit, qui n'est PAS un *fyne.Container (struct à champs
	// Leading/Trailing) : le parcours doit le traverser explicitement.
	treeScroll := NewSmoothVScroll(widget.NewLabel("dossiers"))
	membersScroll := NewSmoothVScroll(widget.NewLabel("membres"))
	treePane := container.NewBorder(widget.NewLabel("arborescence"), nil, nil, nil, treeScroll)
	split := container.NewHSplit(treePane, membersScroll)
	outer := NewSmoothVScroll(container.NewVBox(split))

	adoptNestedScrolls(outer, outer.Scroll.Content)
	if treeScroll.bubbleTo != outer {
		t.Fatal("scroll explorateur derrière HSplit non rattaché à la page")
	}
	if membersScroll.bubbleTo != outer {
		t.Fatal("scroll membres derrière HSplit non rattaché à la page")
	}
}

func TestTeamPageWheelReachesPageThroughSplit(t *testing.T) {
	stubSmoothAsync(t)
	a := test.NewApp()
	defer a.Quit()
	// Réplique fidèle : carte + recherche + HSplit(explorateur court,
	// liste membres) + suite de page. Un cran molette au-dessus de
	// l'explorateur (contenu tenant dans sa vue) doit faire défiler la
	// page, pas mourir dans l'imbriqué.
	treeBox := container.NewVBox(widget.NewLabel("Test"), widget.NewLabel("Test2"))
	treeScroll := NewSmoothVScroll(treeBox)
	treeScroll.SetMinSize(fyne.NewSize(200, 120))
	membersScroll := NewSmoothVScroll(tallBox(3))
	membersScroll.SetMinSize(fyne.NewSize(200, 120))
	split := container.NewHSplit(
		container.NewBorder(widget.NewLabel("Dossiers"), nil, nil, nil, treeScroll),
		membersScroll,
	)
	searchEntry := widget.NewEntry()
	card := createCardBox(container.NewVBox(
		widget.NewLabel("Membres"), searchEntry, split,
	), color.White)
	page := NewPageVScroll(container.NewVBox(card, tallBox(50)))
	w := showSized(a, page, 500, 300)
	defer w.Close()

	if _, ok := entryPassThroughArmed[searchEntry]; !ok {
		t.Fatal("champ recherche non armé")
	}
	page.ScrollToTop()
	treeScroll.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -100)})
	if treeScroll.targetY != 0 {
		t.Fatalf("explorateur a consommé le cran (targetY=%v)", treeScroll.targetY)
	}
	if page.targetY != -100 {
		t.Fatalf("cran non remonté : page targetY=%v, attendu -100", page.targetY)
	}
	driveSmoothToRest(t, page)
	if page.Scroll.Offset.Y <= 0 {
		t.Fatalf("offset page %.1f : la page n'a pas défilé", page.Scroll.Offset.Y)
	}
}

func TestNewPageVScrollWiresNested(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	inner := NewSmoothVScroll(widget.NewLabel("a"))
	entry := widget.NewEntry()
	page := NewPageVScroll(container.NewVBox(inner, entry))
	if inner.bubbleTo != page {
		t.Fatal("NewPageVScroll ne rattache pas les scrolls imbriqués")
	}
	if _, ok := entryPassThroughArmed[entry]; !ok {
		t.Fatal("NewPageVScroll n'arme pas les champs monolignes")
	}
}

// TestPlainEntryShowsInternalScroller documente le mécanisme du bug : un
// Entry par défaut affiche le scroll interne Fyne (renderer entry.go : le
// scroll n'est masqué qu'en WrapOff+ScrollNone). Ce scroll, le plus profond
// dans l'arbre, capte la molette au-dessus du champ. Si Fyne change ce
// comportement, ce test l'indique (le contournement deviendrait inutile).
func TestPlainEntryShowsInternalScroller(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	e := widget.NewEntry()
	if e.Wrapping == fyne.TextWrapOff && e.Scroll == fyne.ScrollNone {
		t.Fatal("Fyne masque désormais le scroll interne par défaut : réévaluer le contournement")
	}
}

func TestEntryPassThroughHidesScrollerWhenTextFits(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	e := widget.NewEntry()
	prevCalls := 0
	e.OnChanged = func(string) { prevCalls++ }
	w := showSized(a, container.NewVBox(e), 400, 200)
	defer w.Close()

	armEntryPassThrough(e)
	if e.Wrapping != fyne.TextWrapOff || e.Scroll != fyne.ScrollNone {
		t.Fatalf("scroll interne non masqué (wrapping=%v scroll=%v)", e.Wrapping, e.Scroll)
	}
	e.SetText("champ.court@domaine.fr")
	if e.Wrapping != fyne.TextWrapOff || e.Scroll != fyne.ScrollNone {
		t.Fatalf("texte court : scroll réapparu (wrapping=%v scroll=%v)", e.Wrapping, e.Scroll)
	}
	if prevCalls != 1 {
		t.Fatalf("OnChanged d'origine non relayé (%d appels)", prevCalls)
	}
	// Idempotence : réarmer ne chaîne pas deux fois.
	armEntryPassThrough(e)
	e.SetText("autre@domaine.fr")
	if prevCalls != 2 {
		t.Fatalf("réarmement : OnChanged relayé %d fois au lieu de 2", prevCalls)
	}
}

func TestEntryPassThroughRestoresScrollerWhenOverflowing(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	e := widget.NewEntry()
	w := showSized(a, container.NewVBox(e), 300, 200)
	defer w.Close()

	armEntryPassThrough(e)
	e.SetText(strings.Repeat("adresse.tres.longue@", 12) + "domaine.fr")
	if e.Wrapping != fyne.TextWrap(fyne.TextTruncateClip) {
		t.Fatalf("texte débordant : wrapping=%v, attendu TruncateClip restauré", e.Wrapping)
	}
	// Retour sous le seuil : le scroll se masque à nouveau (la molette
	// repasse à la page).
	e.SetText("court")
	if e.Wrapping != fyne.TextWrapOff || e.Scroll != fyne.ScrollNone {
		t.Fatalf("retour au court : scroll non remasqué (wrapping=%v scroll=%v)", e.Wrapping, e.Scroll)
	}
}

func TestEntryPassThroughSkipsMultiLine(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	e := widget.NewMultiLineEntry()
	w := showSized(a, container.NewVBox(e), 300, 200)
	defer w.Close()

	armEntryPassThrough(e)
	if _, ok := entryPassThroughArmed[e]; ok {
		t.Fatal("multiligne armé : son scroll vertical légitime serait cassé")
	}
}

package main

// Remontée de la molette vers le scroll de page.
//
// Fyne route chaque cran au Scrollable le plus profond sous le curseur, sans
// remonter au parent : au-dessus d'un scroll imbriqué (mini explorateur,
// liste des membres) ou d'un champ texte (dont le scroll interne Fyne suit
// le curseur), la page ne défilait plus. NewPageVScroll coiffe le contenu
// d'une page : les scrolls imbriqués lui font remonter les crans qu'ils ne
// peuvent pas honorer, et les champs monolignes masquent leur scroll interne
// tant que leur texte tient dans le champ (la molette traverse alors
// jusqu'à la page).
//
// Tout s'exécute sur le fil principal Fyne (construction des pages,
// Scrolled, OnChanged) : aucun verrou n'est nécessaire.

import (
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewPageVScroll coiffe le contenu d'une page (ou d'un dialogue défilant)
// et y branche la remontée molette : scrolls imbriqués rattachés,
// champs monolignes traversants.
func NewPageVScroll(content fyne.CanvasObject) *smoothScroll {
	outer := NewSmoothVScroll(content)
	if content != nil {
		adoptNestedScrolls(outer, content)
		attachEntryPassThrough(content)
	}
	return outer
}

// adoptNestedScrolls rattache chaque smoothScroll descendant de root au
// scroll de page : un cran que l'imbriqué ne peut pas honorer remonte au
// parent au lieu d'être avalé. L'ancêtre retenu est le smoothScroll le plus
// proche (chaînage naturel quand un scroll vit dans le contenu d'un autre).
func adoptNestedScrolls(outer *smoothScroll, root fyne.CanvasObject) {
	var walk func(obj fyne.CanvasObject, parent *smoothScroll)
	walk = func(obj fyne.CanvasObject, parent *smoothScroll) {
		switch o := obj.(type) {
		case *smoothScroll:
			if o == outer {
				// Par construction outer coiffe root ; ne pas redescendre.
				return
			}
			o.bubbleTo = parent
			if o.Scroll != nil && o.Scroll.Content != nil {
				walk(o.Scroll.Content, o)
			}
		case *fyne.Container:
			for _, child := range o.Objects {
				walk(child, parent)
			}
		}
	}
	walk(root, outer)
}

// entryScrollMode mémorise le mode de scroll d'origine d'un champ, pour le
// restaurer quand son texte déborde à nouveau.
type entryScrollMode struct {
	wrapping fyne.TextWrap
	scroll   fyne.ScrollDirection
}

// entryPassThroughArmed garde les champs armés (idempotence + originaux).
// Les champs sont recréés à chaque reconstruction de page ; les pointeurs
// orphelins y restent (quelques dizaines par session au pire : négligeable).
var entryPassThroughArmed = map[*widget.Entry]entryScrollMode{}

// attachEntryPassThrough arme les champs monolignes de l'arbre : leur scroll
// interne Fyne se masque tant que le texte tient (la molette traverse
// jusqu'à la page) et reparaît en cas de débordement (suivi du curseur).
// Les multilignes sont ignorés : leur scroll vertical est légitime.
func attachEntryPassThrough(root fyne.CanvasObject) {
	if root == nil {
		return
	}
	var walk func(obj fyne.CanvasObject)
	walk = func(obj fyne.CanvasObject) {
		switch o := obj.(type) {
		case *widget.Entry:
			armEntryPassThrough(o)
		case *smoothScroll:
			if o.Scroll != nil && o.Scroll.Content != nil {
				walk(o.Scroll.Content)
			}
		case *fyne.Container:
			for _, child := range o.Objects {
				walk(child)
			}
		}
	}
	walk(root)
}

// armEntryPassThrough arme un champ monoligne (idempotent) : l'original est
// mémorisé, OnChanged est chaîné (le callback d'origine est préservé) et le
// mode initial est appliqué aussitôt.
func armEntryPassThrough(e *widget.Entry) {
	if e == nil || e.MultiLine {
		return
	}
	if _, ok := entryPassThroughArmed[e]; ok {
		return
	}
	entryPassThroughArmed[e] = entryScrollMode{wrapping: e.Wrapping, scroll: e.Scroll}
	prev := e.OnChanged
	e.OnChanged = func(text string) {
		updateEntryPassThrough(e)
		if prev != nil {
			prev(text)
		}
	}
	updateEntryPassThrough(e)
}

// updateEntryPassThrough masque le scroll interne Fyne (WrapOff+ScrollNone :
// le renderer entry.go le retire alors de l'arbre) quand le texte tient
// dans le champ, et restaure le mode d'origine sinon (suivi du curseur).
func updateEntryPassThrough(e *widget.Entry) {
	orig, ok := entryPassThroughArmed[e]
	if !ok {
		return
	}
	wantWrapping, wantScroll := orig.wrapping, orig.scroll
	if !entryTextOverflowsWidth(e) {
		wantWrapping, wantScroll = fyne.TextWrapOff, fyne.ScrollNone
	}
	if e.Wrapping != wantWrapping || e.Scroll != wantScroll {
		e.Wrapping = wantWrapping
		e.Scroll = wantScroll
		e.Refresh()
	}
}

// entryTextOverflowsWidth estime si le texte dépasse la largeur visible.
// Sans mise en page (largeur nulle), on suppose que ça tient : la molette
// passe à la page, et le mode est réévalué à la première frappe. Sans
// appli (pas de métriques de thème), on préserve le statu quo. La marge
// ci-dessous surestime le chrome pour ne masquer que lorsque le texte
// tient largement.
func entryTextOverflowsWidth(e *widget.Entry) bool {
	w := e.Size().Width
	if w <= 0 {
		return false
	}
	app := fyne.CurrentApp()
	if app == nil {
		return true
	}
	th := app.Settings().Theme()
	text := e.Text
	if e.Password {
		// Les mots de passe affichent un point par rune, plus large.
		text = strings.Repeat("•", utf8.RuneCountInString(text))
	}
	tw := fyne.MeasureText(text, th.Size(theme.SizeNameText), e.TextStyle)
	return tw.Width+entryPassThroughMargin(e, th) >= w
}

// entryPassThroughMargin surestime le chrome horizontal du champ (paddings,
// place du curseur, icônes action/validation/décor) : en cas de doute le
// scroll interne reste visible (statu quo, suivi curseur préservé) plutôt
// que de clipper le texte.
func entryPassThroughMargin(e *widget.Entry, th fyne.Theme) float32 {
	lineSpace := th.Size(theme.SizeNameLineSpacing)
	margin := th.Size(theme.SizeNameInnerPadding)*2 +
		lineSpace*2 +
		th.Size(theme.SizeNameText)
	iconSpace := th.Size(theme.SizeNameInlineIcon) + lineSpace
	if e.ActionItem != nil {
		margin += iconSpace
	}
	if e.Validator != nil {
		margin += iconSpace
	}
	if e.Icon != nil {
		margin += iconSpace
	}
	return margin
}

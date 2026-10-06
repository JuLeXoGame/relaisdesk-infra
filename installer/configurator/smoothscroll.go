package main

// Défilement molette adouci (Windows) : le driver Fyne déplace le contenu
// de 25 px par cran d'un coup sec, avec un multiplicateur x125 en rotation
// rapide — d'où un défilement saccadé qui cogne en butée. Ce wrapper accumule
// les deltas de la molette et les rejoue en petites étapes (~60 img/s), avec
// une inertie bornée qui préserve la vitesse des gestes rapides.
//
// Deux pièges évités :
//   - chaque étape déplace le contenu directement (Move, sans parcours
//     d'arbre) : passer par Scroll.Scrolled coûterait 5 parcours complets
//     de l'arbre (MinSize) par étape, et Scroll.Refresh relayouterait tout
//     le contenu — les deux saturent le thread principal sur les longues
//     listes (parc, membres) et figent le geste. Barres et ombres sont
//     resynchronisées toutes les 3 étapes (~20 img/s, largement suffisant
//     pour un pouce de scrollbar) et à chaque butée ;
//   - la borne par évènement (720 px) et le plafond cumulé (3600 px)
//     laissent un élan franc parcourir ~un écran en continu, au lieu de
//     cogner en butée (Fyne d'origine) ou de ramper par bonds de 120 px
//     suivis d'un arrêt net.
//
// Tout le pilotage s'exécute sur le thread principal Fyne : Scrolled est
// appelé par le driver sur ce thread, et chaque étape est replanifiée via
// fyne.Do, donc aucun verrou n'est nécessaire.

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

const (
	// smoothScrollStepInterval cadence les étapes d'animation (~60 img/s).
	smoothScrollStepInterval = 16 * time.Millisecond
	// smoothScrollApproach est la fraction de la distance restante
	// parcourue à chaque étape (convergence exponentielle).
	smoothScrollApproach = float32(0.35)
	// smoothScrollSnap termine l'animation quand l'étape calculée passe
	// sous ce seuil : le restant est soldé d'un coup (évite une asymptote
	// infinie).
	smoothScrollSnap = float32(0.5)
	// smoothScrollMaxDelta borne la contribution d'un seul évènement
	// molette : les rafales d'accélération (x125) restent rapides mais
	// continues (~un écran) au lieu de téléporter le contenu en butée.
	smoothScrollMaxDelta = float32(720)
	// smoothScrollMaxPending borne le cumul restant à rejouer : l'inertie
	// d'un geste soutenu est préservée (~5 écrans max) sans emballement.
	smoothScrollMaxPending = float32(3600)
	// smoothScrollSyncEvery resynchronise barres et ombres toutes les N
	// étapes (~20 img/s) : les étapes courantes déplacent le contenu seul,
	// sans parcours d'arbre.
	smoothScrollSyncEvery = 3
)

// smoothScrollAfter planifie la prochaine étape. Variable (et non appel
// direct à time.AfterFunc) pour permettre aux tests de neutraliser
// l'asynchrone et de piloter les étapes à la main, de façon déterministe.
var smoothScrollAfter = func(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

// smoothScroll est un container.Scroll à défilement animé.
type smoothScroll struct {
	*container.Scroll
	targetX, targetY float32 // deltas restants à rejouer
	animating        bool
	cachedMin        fyne.Size // MinSize du contenu, capturée au début du geste
	minValid         bool
	steps            int // étapes du geste courant (cadence la synchro barres)
	// bubbleTo reçoit les crans molette que ce scroll ne peut pas honorer
	// (contenu trop court ou butée atteinte dans le sens du geste) : Fyne
	// route la molette au Scrollable le plus profond sans remonter au
	// parent, donc sans ce relais un scroll imbriqué avale le cran et la
	// page ne défile plus. Nil = pas de parent (scroll de page ou isolé).
	bubbleTo *smoothScroll
}

// newSmoothScroll construit le scroll en une fois : le wrapper EST le widget
// visible (c'est lui qui figure dans l'arbre), donc l'extension Fyne doit le
// désigner dès la construction. Sans cela (double renderer), Refresh mettrait
// à jour un renderer orphelin et les barres affichées ne suivraient jamais.
func newSmoothScroll(direction container.ScrollDirection, content fyne.CanvasObject) *smoothScroll {
	inner := &container.Scroll{Direction: direction, Content: content}
	w := &smoothScroll{Scroll: inner}
	inner.ExtendBaseWidget(w)
	return w
}

// NewSmoothVScroll crée un scroll vertical à défilement adouci.
func NewSmoothVScroll(content fyne.CanvasObject) *smoothScroll {
	return newSmoothScroll(container.ScrollVerticalOnly, content)
}

// NewSmoothHScroll crée un scroll horizontal à défilement adouci.
func NewSmoothHScroll(content fyne.CanvasObject) *smoothScroll {
	return newSmoothScroll(container.ScrollHorizontalOnly, content)
}

// clampScrollDelta borne un delta molette pour dompter les rafales
// d'accélération du driver tout en laissant passer les petits deltas
// (pavés tactiles) et les crans normaux intacts.
func clampScrollDelta(d float32) float32 {
	if d > smoothScrollMaxDelta {
		return smoothScrollMaxDelta
	}
	if d < -smoothScrollMaxDelta {
		return -smoothScrollMaxDelta
	}
	return d
}

// clampPending borne le cumul restant à rejouer (inertie maximale).
func clampPending(v float32) float32 {
	if v > smoothScrollMaxPending {
		return smoothScrollMaxPending
	}
	if v < -smoothScrollMaxPending {
		return -smoothScrollMaxPending
	}
	return v
}

// smoothStepDelta calcule la portion du restant rejouée à cette étape.
// Pure (sans état) pour rester testable sans application Fyne.
func smoothStepDelta(remaining float32) float32 {
	if remaining == 0 {
		return 0
	}
	d := remaining * smoothScrollApproach
	if d > -smoothScrollSnap && d < smoothScrollSnap {
		return remaining // fin : on solde pour éviter l'asymptote
	}
	return d
}

// clampScrollOffset reproduit le cadrage Fyne : l'offset reste dans
// [0, inner-outer], ramené à 0 quand le contenu tient dans la vue.
func clampScrollOffset(start, delta, outer, inner float32) float32 {
	offset := start + delta
	if offset+outer >= inner {
		offset = inner - outer
	}
	return fyne.Max(offset, 0)
}

// clampedOffset applique (dx, dy) à l'offset courant avec le cadrage Fyne,
// à partir de la MinSize capturée (sans reparcourir l'arbre).
func clampedOffset(cur fyne.Position, dx, dy float32, size, min fyne.Size) fyne.Position {
	if min.Width <= size.Width && min.Height <= size.Height {
		return fyne.NewPos(0, 0) // contenu plus petit que la vue : aucune position
	}
	return fyne.NewPos(
		clampScrollOffset(cur.X, -dx, size.Width, min.Width),
		clampScrollOffset(cur.Y, -dy, size.Height, min.Height),
	)
}

// Scrolled accumule le delta molette et démarre l'animation si besoin.
// Le driver Fyne appelle cette méthode sur le thread principal.
func (s *smoothScroll) Scrolled(ev *fyne.ScrollEvent) {
	s.snapStaleOffset()
	if s.Scroll.Direction == container.ScrollNone {
		if s.bubbleTo != nil {
			s.bubbleTo.Scrolled(ev)
		}
		return
	}
	if s.bubbleTo != nil && !s.canHonorScroll(ev.Scrolled.DX, ev.Scrolled.DY) {
		// L'imbriqué ne peut pas honorer ce cran : il remonte au parent
		// (chaînage par évènement, comme les navigateurs) au lieu d'être
		// avalé. L'évènement d'origine est transmis : le parent borne.
		s.bubbleTo.Scrolled(ev)
		return
	}
	s.targetX = clampPending(s.targetX + clampScrollDelta(ev.Scrolled.DX))
	s.targetY = clampPending(s.targetY + clampScrollDelta(ev.Scrolled.DY))
	if !s.animating {
		s.animating = true
		s.steps = 0
		// Capture unique par geste : le cadrage des étapes réutilise cette
		// mesure au lieu de reparcourir l'arbre à chaque étape (le parcours
		// coûte plusieurs ms sur les longues listes et figeait le geste).
		// step() rafraîchit le cache si une butée semble suspecte.
		if s.Scroll.Content != nil {
			s.cachedMin = s.Scroll.Content.MinSize()
			s.minValid = true
		}
		s.scheduleStep()
	}
}

func (s *smoothScroll) scheduleStep() {
	smoothScrollAfter(smoothScrollStepInterval, func() { fyne.Do(s.step) })
}

// snapStaleOffset recadre l'offset dans l'intervalle valide quand le contenu
// a rétréci depuis (liste filtrée) : parité avec updateOffset Fyne, qui
// rabat à chaque cran. Sans cela, un offset hérité pointant dans le vide
// afficherait du vide, la remontée court-circuitant l'animation qui
// recadrait. Coût : une mesure MinSize par cran (Fyne en fait deux) ;
// le Refresh n'a lieu qu'en cas de recadrage effectif.
func (s *smoothScroll) snapStaleOffset() {
	if s.Scroll.Content == nil {
		return
	}
	size := s.Scroll.Size()
	min := s.Scroll.Content.MinSize()
	want := fyne.NewPos(
		clampScrollOffset(s.Scroll.Offset.X, 0, size.Width, min.Width),
		clampScrollOffset(s.Scroll.Offset.Y, 0, size.Height, min.Height),
	)
	if want != s.Scroll.Offset {
		s.Scroll.ScrollToOffset(want)
	}
}

// canHonorScroll prédit si ce scroll bougerait sous le cran (dx, dy),
// en miroir exact de la règle Fyne (scrollBy + cadrage) : conversion
// molette verticale → horizontale sur scroll étroit, puis test par axe.
// Pure lecture (une mesure MinSize, comme Fyne à chaque cran).
func (s *smoothScroll) canHonorScroll(dx, dy float32) bool {
	if s.Scroll.Content == nil {
		return false
	}
	min := s.Scroll.Content.MinSize()
	size := s.Scroll.Size()
	if size.Width < min.Width && size.Height >= min.Height && dx == 0 {
		dx, dy = dy, dx
	}
	off := s.Scroll.Offset
	return canAxisMove(off.X, dx, size.Width, min.Width) ||
		canAxisMove(off.Y, dy, size.Height, min.Height)
}

// canAxisMove indique si le delta ferait bouger l'axe : contenu plus grand
// que la vue, et pas en butée dans le sens du geste (l'offset croît quand
// le delta est négatif, convention Fyne : computeOffset(start, -delta)).
func canAxisMove(offset, delta, outer, inner float32) bool {
	if delta == 0 || inner <= outer {
		return false
	}
	if delta > 0 && offset <= 0 {
		return false
	}
	if delta < 0 && offset >= inner-outer {
		return false
	}
	return true
}

// step rejoue une fraction du restant : l'étape courante déplace le contenu
// seul (sans parcours d'arbre), et toutes les smoothScrollSyncEvery étapes
// une synchro complète repositionne aussi barres et ombres.
// Exécuté sur le thread principal.
func (s *smoothScroll) step() {
	dx := smoothStepDelta(s.targetX)
	dy := smoothStepDelta(s.targetY)
	if dx == 0 && dy == 0 {
		s.animating = false
		s.minValid = false
		return
	}
	if !s.minValid && s.Scroll.Content != nil {
		s.cachedMin = s.Scroll.Content.MinSize()
		s.minValid = true
	}
	ox, oy := dx, dy // espace « évènement » : ce qui sera soldé des targets
	if s.minValid {
		size := s.Scroll.Size()
		// Miroir de la règle Fyne (scrollBy) : sur un scroll horizontal dont
		// le contenu tient en hauteur, la molette verticale défile à
		// l'horizontale.
		if size.Width < s.cachedMin.Width && size.Height >= s.cachedMin.Height && ox == 0 {
			dx, dy = oy, ox
		}
	}
	before := s.Scroll.Offset
	newOffset := clampedOffset(before, dx, dy, s.Scroll.Size(), s.cachedMin)
	if newOffset == before {
		// Butée ? Le cache MinSize peut être périmé si le contenu a changé
		// pendant le geste (liste reconstruite) : un seul nouveau parcours
		// pour vérifier avant de solder.
		if s.minValid && s.Scroll.Content != nil {
			if fresh := s.Scroll.Content.MinSize(); fresh != s.cachedMin {
				s.cachedMin = fresh
				newOffset = clampedOffset(before, dx, dy, s.Scroll.Size(), s.cachedMin)
			}
		}
		if newOffset == before {
			// Vraie butée (ou contenu trop petit) : solder le restant
			// pour ne pas animer dans le vide.
			s.targetX, s.targetY = 0, 0
			s.animating = false
			s.minValid = false
			s.forceSync()
			return
		}
	}
	s.steps++
	if s.steps%smoothScrollSyncEvery == 0 {
		// Synchro complète : barres, ombres et re-cadrage sur mesure fraîche.
		s.Scroll.ScrollToOffset(newOffset)
	} else {
		// Étape courante : déplacement seul, sans parcours d'arbre.
		s.Scroll.Offset = newOffset
		if f := s.Scroll.OnScrolled; f != nil {
			f(newOffset)
		}
		if s.Scroll.Content != nil {
			s.Scroll.Content.Move(fyne.NewPos(-newOffset.X, -newOffset.Y))
		}
	}
	s.targetX -= ox
	s.targetY -= oy
	s.scheduleStep()
}

// forceSync re-synchronise barres et ombres même quand l'offset n'a pas
// bougé (ScrollToOffset ignore les appels sans changement) : le décalage
// transitoire n'est jamais peint, aucun retour au driver entre les deux.
func (s *smoothScroll) forceSync() {
	off := s.Scroll.Offset
	s.Scroll.Offset = fyne.NewPos(off.X+0.5, off.Y+0.5)
	s.Scroll.ScrollToOffset(off)
}

// ScrollToTop recentre en haut et solde l'animation éventuelle
// pour que le restant ne reparte pas aussitôt vers le bas.
func (s *smoothScroll) ScrollToTop() {
	s.Scroll.ScrollToTop()
	s.targetX, s.targetY = 0, 0
}

// ScrollToBottom recentre en bas et solde l'animation éventuelle.
func (s *smoothScroll) ScrollToBottom() {
	s.Scroll.ScrollToBottom()
	s.targetX, s.targetY = 0, 0
}

package main

import (
	"errors"
	"testing"
)

func TestIsFleetReplaceableError(t *testing.T) {
	replaceable := []string{
		"un accès permanent existe déjà ; retirez-le explicitement avec --unenroll avant de le remplacer",
		"un enrôlement est déjà enregistré ; utilisez --unenroll avant de recommencer",
		"installation de parc déjà présente ou interrompue ; utilisez --unenroll avant de recommencer",
	}
	for _, msg := range replaceable {
		if !isFleetReplaceableError(errors.New(msg)) {
			t.Errorf("expected replaceable error for %q", msg)
		}
	}
	unrelated := []error{
		nil,
		errors.New("boom"),
		errors.New("droits administrateur requis"),
		errors.New("un accès permanent est installé ; retirez-le explicitement avant une session temporaire"),
	}
	for _, err := range unrelated {
		if isFleetReplaceableError(err) {
			t.Errorf("expected non-replaceable error for %v", err)
		}
	}
}

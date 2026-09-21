package main

import (
	"reflect"
	"strings"
	"testing"
)

func sampleThreeLevelFolders() []DeviceFolderItem {
	return []DeviceFolderItem{
		{FolderID: "fld-paris", Name: "Siège Paris", ParentFolderID: ""},
		{FolderID: "fld-lyon", Name: "Agence Lyon", ParentFolderID: ""},
		{FolderID: "fld-compta", Name: "Comptabilité", ParentFolderID: "fld-paris"},
		{FolderID: "fld-rh", Name: "Ressources Humaines", ParentFolderID: "fld-paris"},
		{FolderID: "fld-serveurs", Name: "Serveurs", ParentFolderID: "fld-compta"},
		{FolderID: "fld-postes", Name: "Postes de travail", ParentFolderID: "fld-compta"},
	}
}

func TestFolderBreadcrumbsThreeLevels(t *testing.T) {
	folders := sampleThreeLevelFolders()

	// 1. Root & special values
	if path := getFolderBreadcrumbPath("", folders); path != "" {
		t.Errorf("expected empty breadcrumb for root, got %q", path)
	}
	if path := getFolderBreadcrumbPath("ALL", folders); path != "" {
		t.Errorf("expected empty breadcrumb for ALL, got %q", path)
	}
	if depth := getFolderDepth("", folders); depth != 0 {
		t.Errorf("expected depth 0 for root, got %d", depth)
	}

	// 2. Level 1 (Siège Paris)
	anc1 := getFolderAncestors("fld-paris", folders)
	if len(anc1) != 1 || anc1[0].FolderID != "fld-paris" {
		t.Fatalf("unexpected ancestors for level 1: %+v", anc1)
	}
	if path := getFolderBreadcrumbPath("fld-paris", folders); path != "Siège Paris" {
		t.Errorf("unexpected breadcrumb path for level 1: %q", path)
	}
	if depth := getFolderDepth("fld-paris", folders); depth != 1 {
		t.Errorf("expected depth 1 for level 1, got %d", depth)
	}

	// 3. Level 2 (Comptabilité under Siège Paris)
	anc2 := getFolderAncestors("fld-compta", folders)
	if len(anc2) != 2 || anc2[0].FolderID != "fld-paris" || anc2[1].FolderID != "fld-compta" {
		t.Fatalf("unexpected ancestors for level 2: %+v", anc2)
	}
	if path := getFolderBreadcrumbPath("fld-compta", folders); path != "Siège Paris > Comptabilité" {
		t.Errorf("unexpected breadcrumb path for level 2: %q", path)
	}
	if depth := getFolderDepth("fld-compta", folders); depth != 2 {
		t.Errorf("expected depth 2 for level 2, got %d", depth)
	}

	// 4. Level 3 (Serveurs under Comptabilité under Siège Paris)
	anc3 := getFolderAncestors("fld-serveurs", folders)
	if len(anc3) != 3 || anc3[0].FolderID != "fld-paris" || anc3[1].FolderID != "fld-compta" || anc3[2].FolderID != "fld-serveurs" {
		t.Fatalf("unexpected ancestors for level 3: %+v", anc3)
	}
	expectedPath := "Siège Paris > Comptabilité > Serveurs"
	if path := getFolderBreadcrumbPath("fld-serveurs", folders); path != expectedPath {
		t.Errorf("unexpected breadcrumb path for level 3: %q, expected %q", path, expectedPath)
	}
	if depth := getFolderDepth("fld-serveurs", folders); depth != 3 {
		t.Errorf("expected depth 3 for level 3, got %d", depth)
	}
}

func TestFolderDescendantsThreeLevels(t *testing.T) {
	folders := sampleThreeLevelFolders()

	// Level 1: "fld-paris" should include itself, fld-compta, fld-rh, fld-serveurs, fld-postes
	desc1 := getFolderAndDescendantIDs("fld-paris", folders)
	expected1 := map[string]bool{
		"fld-paris":    true,
		"fld-compta":   true,
		"fld-rh":       true,
		"fld-serveurs": true,
		"fld-postes":   true,
	}
	if !reflect.DeepEqual(desc1, expected1) {
		t.Errorf("unexpected descendants for level 1:\ngot:  %+v\nwant: %+v", desc1, expected1)
	}

	// Level 2: "fld-compta" should include itself, fld-serveurs, fld-postes
	desc2 := getFolderAndDescendantIDs("fld-compta", folders)
	expected2 := map[string]bool{
		"fld-compta":   true,
		"fld-serveurs": true,
		"fld-postes":   true,
	}
	if !reflect.DeepEqual(desc2, expected2) {
		t.Errorf("unexpected descendants for level 2:\ngot:  %+v\nwant: %+v", desc2, expected2)
	}

	// Level 3: "fld-serveurs" should only include itself
	desc3 := getFolderAndDescendantIDs("fld-serveurs", folders)
	expected3 := map[string]bool{
		"fld-serveurs": true,
	}
	if !reflect.DeepEqual(desc3, expected3) {
		t.Errorf("unexpected descendants for level 3:\ngot:  %+v\nwant: %+v", desc3, expected3)
	}
}

func TestDirectChildFolders(t *testing.T) {
	folders := sampleThreeLevelFolders()

	// Root direct children: Agence Lyon, Siège Paris (alphabetical)
	rootChildren := getDirectChildFolders("", folders)
	if len(rootChildren) != 2 || rootChildren[0].Name != "Agence Lyon" || rootChildren[1].Name != "Siège Paris" {
		t.Errorf("unexpected root children: %+v", rootChildren)
	}

	// Level 1 direct children: Comptabilité, Ressources Humaines
	l1Children := getDirectChildFolders("fld-paris", folders)
	if len(l1Children) != 2 || l1Children[0].Name != "Comptabilité" || l1Children[1].Name != "Ressources Humaines" {
		t.Errorf("unexpected l1 children: %+v", l1Children)
	}

	// Level 2 direct children: Postes de travail, Serveurs (alphabetical)
	l2Children := getDirectChildFolders("fld-compta", folders)
	if len(l2Children) != 2 || l2Children[0].Name != "Postes de travail" || l2Children[1].Name != "Serveurs" {
		t.Errorf("unexpected l2 children: %+v", l2Children)
	}

	// Level 3 direct children: empty
	l3Children := getDirectChildFolders("fld-serveurs", folders)
	if len(l3Children) != 0 {
		t.Errorf("expected 0 children for level 3, got %+v", l3Children)
	}
}

func TestBuildFolderOptionsIndentation(t *testing.T) {
	folders := sampleThreeLevelFolders()

	optsWithAll := buildFolderOptions(folders, true)
	if len(optsWithAll) < 3 || optsWithAll[0].id != "ALL" || optsWithAll[1].id != "" {
		t.Fatalf("expected options with ALL and Root, got %+v", optsWithAll)
	}

	optsNoAll := buildFolderOptions(folders, false)
	if len(optsNoAll) < 2 || optsNoAll[0].id != "" {
		t.Fatalf("expected options with Root first, got %+v", optsNoAll)
	}

	// Verify indentation for 3 levels:
	// Level 1: "📁 Agence Lyon", "📁 Siège Paris"
	// Level 2: "  ↳ 📁 Comptabilité"
	// Level 3: "    ↳ 📁 Postes de travail", "    ↳ 📁 Serveurs"
	var serveursOpt *folderOption
	var comptaOpt *folderOption
	var parisOpt *folderOption
	for i := range optsNoAll {
		if optsNoAll[i].id == "fld-serveurs" {
			serveursOpt = &optsNoAll[i]
		}
		if optsNoAll[i].id == "fld-compta" {
			comptaOpt = &optsNoAll[i]
		}
		if optsNoAll[i].id == "fld-paris" {
			parisOpt = &optsNoAll[i]
		}
	}

	if parisOpt == nil || !strings.HasPrefix(parisOpt.label, "📁 ") {
		t.Errorf("expected level 1 prefix '📁 ', got %v", parisOpt)
	}
	if comptaOpt == nil || !strings.HasPrefix(comptaOpt.label, "  ↳ 📁 ") {
		t.Errorf("expected level 2 prefix '  ↳ 📁 ', got %v", comptaOpt)
	}
	if serveursOpt == nil || !strings.HasPrefix(serveursOpt.label, "    ↳ 📁 ") {
		t.Errorf("expected level 3 prefix '    ↳ 📁 ', got %v", serveursOpt)
	}
}

func TestFolderCycleProtection(t *testing.T) {
	// A circular parent reference must not enter an infinite loop
	cycleFolders := []DeviceFolderItem{
		{FolderID: "A", Name: "A", ParentFolderID: "B"},
		{FolderID: "B", Name: "B", ParentFolderID: "A"},
	}
	anc := getFolderAncestors("A", cycleFolders)
	if len(anc) == 0 || len(anc) > 2 {
		t.Errorf("expected loop prevention to return at most 2 items, got %d", len(anc))
	}
}

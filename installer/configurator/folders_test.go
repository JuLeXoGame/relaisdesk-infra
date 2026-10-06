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

	opts := buildFolderOptions(folders)
	if len(opts) < 2 || opts[0].id != "" {
		t.Fatalf("expected options with Root first, got %+v", opts)
	}
	for _, o := range opts {
		if o.id == "ALL" {
			t.Fatalf("unexpected ALL option: %+v", opts)
		}
	}
	optsNoAll := opts

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

// La racine affiche tout le parc (fusion de l'ancienne vue « Tous les
// postes ») ; un dossier affiche son sous-arbre.
func TestFilterDevicesByFolder(t *testing.T) {
	folders := sampleThreeLevelFolders()
	devices := []DeviceItem{
		{DeviceID: "d-root", FolderID: ""},
		{DeviceID: "d-paris", FolderID: "fld-paris"},
		{DeviceID: "d-compta", FolderID: "fld-compta"},
		{DeviceID: "d-serveurs", FolderID: "fld-serveurs"},
		{DeviceID: "d-lyon", FolderID: "fld-lyon"},
	}
	ids := func(devs []DeviceItem) []string {
		out := []string{}
		for _, d := range devs {
			out = append(out, d.DeviceID)
		}
		return out
	}
	if got := ids(filterDevicesByFolder(devices, "", folders)); !reflect.DeepEqual(got, []string{"d-root", "d-paris", "d-compta", "d-serveurs", "d-lyon"}) {
		t.Errorf("racine = %v, attendu tout le parc", got)
	}
	if got := ids(filterDevicesByFolder(devices, "fld-paris", folders)); !reflect.DeepEqual(got, []string{"d-paris", "d-compta", "d-serveurs"}) {
		t.Errorf("sous-arbre paris = %v", got)
	}
	if got := ids(filterDevicesByFolder(devices, "fld-compta", folders)); !reflect.DeepEqual(got, []string{"d-compta", "d-serveurs"}) {
		t.Errorf("sous-arbre compta = %v", got)
	}
	if got := filterDevicesByFolder(devices, "fld-inconnu", folders); len(got) != 0 {
		t.Errorf("dossier inconnu = %v, attendu vide", ids(got))
	}
	if got := filterDevicesByFolder(nil, "", folders); len(got) != 0 {
		t.Errorf("parc vide racine = %v", ids(got))
	}
}

func TestFilterMembersByFolder(t *testing.T) {
	folders := sampleThreeLevelFolders()
	members := []customerTeamMember{
		{MemberID: "m-none", FolderIDs: nil},
		{MemberID: "m-paris", FolderIDs: []string{"fld-paris"}},
		{MemberID: "m-multi", FolderIDs: []string{"fld-lyon", "fld-serveurs"}},
		{MemberID: "m-unknown", FolderIDs: []string{"fld-inconnu"}},
	}
	ids := func(ms []customerTeamMember) []string {
		out := []string{}
		for _, m := range ms {
			out = append(out, m.MemberID)
		}
		return out
	}
	if got := ids(filterMembersByFolder(members, "", folders)); !reflect.DeepEqual(got, []string{"m-none", "m-paris", "m-multi", "m-unknown"}) {
		t.Errorf("racine = %v, attendu tous les membres", got)
	}
	if got := ids(filterMembersByFolder(members, "fld-paris", folders)); !reflect.DeepEqual(got, []string{"m-paris", "m-multi"}) {
		t.Errorf("sous-arbre paris = %v (m-multi via fld-serveurs)", got)
	}
	if got := ids(filterMembersByFolder(members, "fld-lyon", folders)); !reflect.DeepEqual(got, []string{"m-multi"}) {
		t.Errorf("lyon = %v", got)
	}
	if got := filterMembersByFolder(members, "fld-absent", folders); len(got) != 0 {
		t.Errorf("dossier absent = %v, attendu vide", ids(got))
	}
	if got := filterMembersByFolder(nil, "", folders); len(got) != 0 {
		t.Errorf("membres vides racine = %v", ids(got))
	}
}

func TestCustomerFoldersConversion(t *testing.T) {
	in := []customerFolder{
		{FolderID: "a", Name: "A", ParentFolderID: "", LicenseID: "lic-1"},
		{FolderID: "b", Name: "B", ParentFolderID: "a", LicenseID: "lic-1"},
	}
	out := customerFoldersToDeviceFolders(in)
	if len(out) != 2 || out[0].FolderID != "a" || out[0].Name != "A" || out[1].ParentFolderID != "a" || out[1].LicenseID != "lic-1" {
		t.Fatalf("conversion inattendue: %+v", out)
	}
	if got := getDirectChildFolders("", out); len(got) != 1 || got[0].FolderID != "a" {
		t.Errorf("helpers partagés inutilisables après conversion: %+v", got)
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

func TestBuildFolderMoveOptionsExcludesSubtree(t *testing.T) {
	folders := []DeviceFolderItem{
		{FolderID: "A", Name: "Agence", ParentFolderID: ""},
		{FolderID: "B", Name: "Compta", ParentFolderID: "A"},
		{FolderID: "C", Name: "Factures", ParentFolderID: "B"},
		{FolderID: "D", Name: "Dépôt", ParentFolderID: ""},
	}
	options := buildFolderMoveOptions(folders, "B")
	ids := map[string]bool{}
	for _, o := range options {
		ids[o.id] = true
	}
	if ids["B"] || ids["C"] {
		t.Fatalf("le sous-arbre déplacé doit être exclu: %+v", options)
	}
	if !ids[""] || !ids["A"] || !ids["D"] {
		t.Fatalf("destinations valides manquantes: %+v", options)
	}
}

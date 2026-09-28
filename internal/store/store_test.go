package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setupProfile(t *testing.T, root, name string) {
	t.Helper()
	libDir := filepath.Join(root, name, "libraries")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "x_library"), []byte("a|echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProfile_DefaultSeedsVarsFile(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "default")

	p, err := LoadProfile(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"VSS", "KVNR_LIST"}
	if !reflect.DeepEqual(p.MustHave, want) {
		t.Fatalf("MustHave = %v, want %v", p.MustHave, want)
	}

	// vars.yaml and favorites.yaml should now exist on disk.
	if _, err := os.Stat(filepath.Join(root, "default", "vars.yaml")); err != nil {
		t.Fatalf("vars.yaml not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "default", "favorites.yaml")); err != nil {
		t.Fatalf("favorites.yaml not created: %v", err)
	}
}

func TestLoadProfile_NonDefaultGetsEmptyMustHave(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "staging")

	p, err := LoadProfile(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.MustHave) != 0 {
		t.Fatalf("MustHave = %v, want empty", p.MustHave)
	}
}

func TestLoadProfile_MissingDirErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadProfile(root, "nope"); err == nil {
		t.Fatal("expected error for missing profile dir, got nil")
	}
}

func TestFavoritesToggleAndPersist(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "default")

	p, err := LoadProfile(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	if p.Favorites.IsFavorite("x_library", "a") {
		t.Fatal("should not start favorited")
	}
	if fav := p.Favorites.Toggle("x_library", "a"); !fav {
		t.Fatal("Toggle should report now-favorited")
	}
	if err := p.SaveFavorites(); err != nil {
		t.Fatal(err)
	}

	p2, err := LoadProfile(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	if !p2.Favorites.IsFavorite("x_library", "a") {
		t.Fatal("favorite did not persist across reload")
	}
}

func TestHistoryRoundTrip_PreservesVarOrder(t *testing.T) {
	root := t.TempDir()
	h := History{
		Profile: "default",
		Library: "connect_library",
		Script:  "Prod-BE-Z1",
		Vars: VarList{
			{Name: "VSS", Value: "v1"},
			{Name: "KVNR_LIST", Value: "v2"},
			{Name: "SOME_ADHOC_VAR", Value: "v3"},
		},
	}
	if err := h.Save(root); err != nil {
		t.Fatal(err)
	}

	got, err := LoadHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != h.Profile || got.Library != h.Library || got.Script != h.Script {
		t.Fatalf("history mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Vars, h.Vars) {
		t.Fatalf("vars order/content mismatch: got %+v, want %+v", got.Vars, h.Vars)
	}
}

func TestLoadHistory_MissingFileReturnsZeroValue(t *testing.T) {
	root := t.TempDir()
	h, err := LoadHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if h.Profile != "" {
		t.Fatalf("expected empty profile, got %q", h.Profile)
	}
}

func TestListProfiles(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "default")
	setupProfile(t, root, "staging")
	if err := os.MkdirAll(filepath.Join(root, "not_a_profile"), 0o755); err != nil {
		t.Fatal(err)
	}

	names, err := ListProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"default", "staging"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("ListProfiles = %v, want %v", names, want)
	}
}

func TestLoadProfile_OptionalVarsWithDefaultsAndChoices(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "p")
	vars := `must_have:
  - VSS
  - name: ZONE
    choices: [z1, z2]
optional:
  - name: ENV
    default: prod
    choices: [prod, preprod]
  - TENANT
  - VSS
`
	if err := os.WriteFile(filepath.Join(root, "p", "vars.yaml"), []byte(vars), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadProfile(root, "p")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"VSS", "ZONE"}; !reflect.DeepEqual(p.MustHave, want) {
		t.Fatalf("MustHave = %v, want %v", p.MustHave, want)
	}
	// VSS is already a must-have, so its optional duplicate is ignored.
	if want := []string{"ENV", "TENANT"}; !reflect.DeepEqual(p.Optional, want) {
		t.Fatalf("Optional = %v, want %v", p.Optional, want)
	}
	if d := p.Def("ENV"); d.Default != "prod" || !reflect.DeepEqual(d.Choices, []string{"prod", "preprod"}) {
		t.Fatalf("Def(ENV) = %+v", d)
	}
	if d := p.Def("ZONE"); !reflect.DeepEqual(d.Choices, []string{"z1", "z2"}) {
		t.Fatalf("Def(ZONE) = %+v", d)
	}
	if d := p.Def("UNDECLARED"); d.Default != "" || d.Choices != nil {
		t.Fatalf("Def(UNDECLARED) = %+v, want zero", d)
	}
}

func TestLoadProfile_VarDefMissingNameErrors(t *testing.T) {
	root := t.TempDir()
	setupProfile(t, root, "p")
	if err := os.WriteFile(filepath.Join(root, "p", "vars.yaml"), []byte("optional:\n  - default: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(root, "p"); err == nil {
		t.Fatal("expected error for definition without a name")
	}
}

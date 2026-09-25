package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFile_SingleLineEntries(t *testing.T) {
	content := "check_minimal|check_minimal {{TENANT}} {{KVNR}} {{ENV}}\n\n" +
		"fast_check|fast_check {{TENANT}} {{KVNR}} {{ENV}}\n"
	path := writeTemp(t, "check_library", content)

	lib, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Label != "check" {
		t.Fatalf("label = %q, want %q", lib.Label, "check")
	}
	if len(lib.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(lib.Entries))
	}
	e := lib.Entries[0]
	if e.Name != "check_minimal" {
		t.Fatalf("name = %q", e.Name)
	}
	wantCmd := "check_minimal {{TENANT}} {{KVNR}} {{ENV}}"
	if e.RawCommand != wantCmd {
		t.Fatalf("raw = %q, want %q", e.RawCommand, wantCmd)
	}
	if e.FlatCommand != wantCmd {
		t.Fatalf("flat = %q, want %q", e.FlatCommand, wantCmd)
	}
	wantPH := []string{"TENANT", "KVNR", "ENV"}
	if !reflect.DeepEqual(e.Placeholders, wantPH) {
		t.Fatalf("placeholders = %v, want %v", e.Placeholders, wantPH)
	}
}

func TestParseFile_MultiLineContinuation(t *testing.T) {
	content := "get_json|curl -X GET \"{{URL}}\" \\\n" +
		"  -H \"Accept: application/json\" -d \"{{ENV}}:{{KVNR}}:{{TENANT}}\"\n" +
		"\n" +
		"get_headers|curl -I \"{{URL}}\"\n"
	path := writeTemp(t, "curl_library", content)

	lib, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(lib.Entries))
	}

	e := lib.Entries[0]
	if e.Name != "get_json" {
		t.Fatalf("name = %q", e.Name)
	}

	wantRaw := "curl -X GET \"{{URL}}\" \\\n  -H \"Accept: application/json\" -d \"{{ENV}}:{{KVNR}}:{{TENANT}}\""
	if e.RawCommand != wantRaw {
		t.Fatalf("raw = %q, want %q", e.RawCommand, wantRaw)
	}

	wantFlat := "curl -X GET \"{{URL}}\"   -H \"Accept: application/json\" -d \"{{ENV}}:{{KVNR}}:{{TENANT}}\""
	if e.FlatCommand != wantFlat {
		t.Fatalf("flat = %q, want %q", e.FlatCommand, wantFlat)
	}

	wantPH := []string{"URL", "ENV", "KVNR", "TENANT"}
	if !reflect.DeepEqual(e.Placeholders, wantPH) {
		t.Fatalf("placeholders = %v, want %v", e.Placeholders, wantPH)
	}

	e2 := lib.Entries[1]
	if e2.Name != "get_headers" || e2.RawCommand != `curl -I "{{URL}}"` {
		t.Fatalf("second entry mismatch: %+v", e2)
	}
}

func TestParseFile_BlankLineSeparation(t *testing.T) {
	// Multiple consecutive blank lines between entries should still just
	// separate them (not produce empty entries).
	content := "a|cmd a\n\n\n\nb|cmd b\n\n\n"
	path := writeTemp(t, "x_library", content)

	lib, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(lib.Entries), lib.Entries)
	}
	if lib.Entries[0].Name != "a" || lib.Entries[1].Name != "b" {
		t.Fatalf("unexpected entries: %+v", lib.Entries)
	}
}

func TestSubstitute(t *testing.T) {
	cmd := "curl {{URL}} -d {{MISSING}}"
	got := Substitute(cmd, map[string]string{"URL": "http://x"})
	want := "curl http://x -d "
	if got != want {
		t.Fatalf("Substitute = %q, want %q", got, want)
	}
}

func TestLoadDir_AlphabeticalOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"curl_library", "check_library", "connect_library"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x|echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	libs, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, l := range libs {
		keys = append(keys, l.Key)
	}
	want := []string{"check_library", "connect_library", "curl_library"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("order = %v, want %v", keys, want)
	}
}

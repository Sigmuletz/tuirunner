package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tuirunner/internal/store"
)

func newTestModel(t *testing.T, mustHave []string) *Model {
	t.Helper()
	profile := &store.Profile{
		Name:      "default",
		MustHave:  mustHave,
		Favorites: store.FavoritesFile{Favorites: map[string][]string{}},
	}
	return NewModel("", profile, store.History{}, nil)
}

func (m *Model) fieldByName(name string) *Field {
	for i := range m.fields {
		if m.fields[i].Name == name {
			return &m.fields[i]
		}
	}
	return nil
}

func TestSyncDerivedFields_CreatesDropdownAndComputed(t *testing.T) {
	m := newTestModel(t, []string{"VSS", "KVNR_LIST"})
	kvnr := m.fieldByName("KVNR_LIST")
	if kvnr == nil {
		t.Fatal("KVNR_LIST must-have field missing")
	}
	kvnr.Input.SetValue("1231, 1231231 ,12312")
	m.syncDerivedFields()

	dd := m.fieldByName("KVNR")
	if dd == nil {
		t.Fatal("expected derived dropdown field KVNR")
	}
	if dd.Kind != FieldDropdown {
		t.Fatalf("KVNR kind = %v, want FieldDropdown", dd.Kind)
	}
	wantOpts := []string{"1231", "1231231", "12312"}
	if len(dd.Options) != len(wantOpts) {
		t.Fatalf("KVNR options = %v, want %v", dd.Options, wantOpts)
	}
	for i, o := range wantOpts {
		if dd.Options[i] != o {
			t.Fatalf("KVNR options[%d] = %q, want %q", i, dd.Options[i], o)
		}
	}
	if got := dd.Input.Value(); got != "1231" {
		t.Fatalf("KVNR value = %q, want %q (first option)", got, "1231")
	}

	sql := m.fieldByName("KVNR_LIST_SQL")
	if sql == nil {
		t.Fatal("expected derived computed field KVNR_LIST_SQL")
	}
	if sql.Kind != FieldComputed {
		t.Fatalf("KVNR_LIST_SQL kind = %v, want FieldComputed", sql.Kind)
	}
	wantSQL := "('1231','1231231','12312')"
	if got := sql.Input.Value(); got != wantSQL {
		t.Fatalf("KVNR_LIST_SQL value = %q, want %q", got, wantSQL)
	}
}

func TestSyncDerivedFields_EmptySourceYieldsEmptyDerived(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.syncDerivedFields()

	dd := m.fieldByName("KVNR")
	if dd == nil || len(dd.Options) != 0 || dd.Input.Value() != "" {
		t.Fatalf("expected empty KVNR dropdown for empty source, got %+v", dd)
	}
	sql := m.fieldByName("KVNR_LIST_SQL")
	if sql == nil || sql.Input.Value() != "" {
		t.Fatalf("expected empty KVNR_LIST_SQL for empty source, got %+v", sql)
	}
}

func TestSyncDerivedFields_LiveUpdateOnEdit(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.fieldByName("KVNR_LIST").Input.SetValue("a,b")
	m.syncDerivedFields()

	if got := m.fieldByName("KVNR_LIST_SQL").Input.Value(); got != "('a','b')" {
		t.Fatalf("after first edit, KVNR_LIST_SQL = %q", got)
	}

	// syncDerivedFields rebuilds m.fields, so re-fetch the source field
	// rather than reusing a pointer captured before that rebuild.
	m.fieldByName("KVNR_LIST").Input.SetValue("a,b,c")
	m.syncDerivedFields()

	if got := m.fieldByName("KVNR_LIST_SQL").Input.Value(); got != "('a','b','c')" {
		t.Fatalf("after second edit, KVNR_LIST_SQL = %q", got)
	}
	if got := len(m.fieldByName("KVNR").Options); got != 3 {
		t.Fatalf("after second edit, KVNR options count = %d, want 3", got)
	}
}

func TestSyncDerivedFields_PreservesSelectionAcrossEdit(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.fieldByName("KVNR_LIST").Input.SetValue("a,b,c")
	m.syncDerivedFields()

	dd := m.fieldByName("KVNR")
	dd.SelectedIndex = 1 // pick "b"
	dd.Input.SetValue("b")

	// Editing the source but keeping "b" among the options should keep
	// "b" selected rather than resetting to the first option. Re-fetch
	// the source field since syncDerivedFields rebuilds m.fields.
	m.fieldByName("KVNR_LIST").Input.SetValue("x,b,y,z")
	m.syncDerivedFields()

	dd = m.fieldByName("KVNR")
	if got := dd.Input.Value(); got != "b" {
		t.Fatalf("selection after edit = %q, want %q (preserved)", got, "b")
	}
}

func TestSyncDerivedFields_DropsOrphansWhenSourceRemoved(t *testing.T) {
	m := newTestModel(t, nil)
	m.addField("FOO_LIST", "1,2", false)
	m.syncDerivedFields()
	if m.fieldByName("FOO") == nil || m.fieldByName("FOO_LIST_SQL") == nil {
		t.Fatal("expected derived fields for ad-hoc FOO_LIST")
	}

	// Remove the source field directly (as Ctrl+D would) and re-sync.
	idx := m.fieldIndex("FOO_LIST")
	m.fields = append(m.fields[:idx], m.fields[idx+1:]...)
	m.syncDerivedFields()

	if m.fieldByName("FOO") != nil || m.fieldByName("FOO_LIST_SQL") != nil {
		t.Fatal("expected derived fields to be dropped once their source is removed")
	}
}

func TestSyncDerivedFields_DoesNotClobberIndependentField(t *testing.T) {
	m := newTestModel(t, nil)
	m.addField("KVNR", "manual-value", false) // user-created, unrelated
	m.addField("KVNR_LIST", "1,2", false)
	m.syncDerivedFields()

	kvnr := m.fieldByName("KVNR")
	if kvnr.Kind != FieldText || kvnr.Input.Value() != "manual-value" {
		t.Fatalf("independent KVNR field was clobbered: %+v", kvnr)
	}
}

func TestHandleInputKey_LeftRightCyclesDropdown(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	kvnr := m.fieldByName("KVNR_LIST")
	kvnr.Input.SetValue("a,b,c")
	m.syncDerivedFields()

	m.focus = FocusInput
	m.inputIndex = m.fieldIndex("KVNR")
	m.focusField(m.inputIndex)

	m.handleInputKey(tea.KeyMsg{Type: tea.KeyRight})
	if got := m.fieldByName("KVNR").Input.Value(); got != "b" {
		t.Fatalf("after right, KVNR = %q, want %q", got, "b")
	}
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyRight})
	if got := m.fieldByName("KVNR").Input.Value(); got != "c" {
		t.Fatalf("after second right, KVNR = %q, want %q", got, "c")
	}
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyRight})
	if got := m.fieldByName("KVNR").Input.Value(); got != "c" {
		t.Fatalf("right at last option should clamp, got %q", got)
	}
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyLeft})
	if got := m.fieldByName("KVNR").Input.Value(); got != "b" {
		t.Fatalf("after left, KVNR = %q, want %q", got, "b")
	}
}

func TestHandleInputKey_CtrlDNoOpOnDerivedFields(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.fieldByName("KVNR_LIST").Input.SetValue("a,b")
	m.syncDerivedFields()

	before := len(m.fields)
	m.focus = FocusInput
	m.inputIndex = m.fieldIndex("KVNR")
	m.focusField(m.inputIndex)
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if len(m.fields) != before {
		t.Fatalf("Ctrl+D on a FieldDropdown should be a no-op, field count changed %d -> %d", before, len(m.fields))
	}

	m.inputIndex = m.fieldIndex("KVNR_LIST_SQL")
	m.focusField(m.inputIndex)
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if len(m.fields) != before {
		t.Fatalf("Ctrl+D on a FieldComputed should be a no-op, field count changed %d -> %d", before, len(m.fields))
	}
}

func TestEnsureInputFocusValid_SkipsComputedField(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.fieldByName("KVNR_LIST").Input.SetValue("a")
	m.syncDerivedFields()

	// Force inputIndex onto the read-only computed field, as if Tab
	// landed there, and confirm ensureInputFocusValid steps off it.
	m.inputIndex = m.fieldIndex("KVNR_LIST_SQL")
	m.ensureInputFocusValid()
	if m.fields[m.inputIndex].Kind == FieldComputed {
		t.Fatalf("ensureInputFocusValid left focus on a FieldComputed field")
	}
}

func TestRestoreDropdownSelections(t *testing.T) {
	m := newTestModel(t, []string{"KVNR_LIST"})
	m.fieldByName("KVNR_LIST").Input.SetValue("a,b,c")
	m.syncDerivedFields()

	vars := store.VarList{{Name: "KVNR", Value: "c"}}
	m.restoreDropdownSelections(vars)

	dd := m.fieldByName("KVNR")
	if dd.Input.Value() != "c" || dd.SelectedIndex != 2 {
		t.Fatalf("restoreDropdownSelections did not select %q: %+v", "c", dd)
	}
}

func newDefsModel(t *testing.T, hist store.History) *Model {
	t.Helper()
	profile := &store.Profile{
		Name:      "default",
		MustHave:  []string{"VSS"},
		Optional:  []string{"ENV", "TENANT"},
		Favorites: store.FavoritesFile{Favorites: map[string][]string{}},
		Defs: map[string]store.VarDef{
			"VSS":    {Name: "VSS", Default: "v1"},
			"ENV":    {Name: "ENV", Default: "prod", Choices: []string{"prod", "preprod", "dev"}},
			"TENANT": {Name: "TENANT"},
		},
	}
	return NewModel("", profile, hist, nil)
}

func TestOptionalFields_PresentWithDefaults(t *testing.T) {
	m := newDefsModel(t, store.History{})
	for name, want := range map[string]string{"VSS": "v1", "ENV": "prod", "TENANT": ""} {
		f := m.fieldByName(name)
		if f == nil {
			t.Fatalf("field %s missing", name)
		}
		if got := f.Input.Value(); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if f := m.fieldByName("ENV"); !f.Optional || f.MustHave || len(f.Choices) != 3 {
		t.Fatalf("ENV field = %+v", f)
	}
}

func TestOptionalFields_HistoryWinsOverDefault(t *testing.T) {
	m := newDefsModel(t, store.History{Vars: store.VarList{{Name: "ENV", Value: "custom"}}})
	if got := m.valueOf("ENV"); got != "custom" {
		t.Fatalf("ENV = %q, want history value %q", got, "custom")
	}
}

func TestHandleInputKey_UpDownCyclesChoicesButStaysFreeText(t *testing.T) {
	m := newDefsModel(t, store.History{})
	m.focus = FocusInput
	m.focusField(m.fieldIndex("ENV"))

	m.handleInputKey(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.valueOf("ENV"); got != "preprod" {
		t.Fatalf("after down, ENV = %q, want preprod", got)
	}
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyDown})
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyDown}) // clamps at last
	if got := m.valueOf("ENV"); got != "dev" {
		t.Fatalf("after down x3, ENV = %q, want dev", got)
	}

	// Free text is still allowed.
	m.fieldByName("ENV").Input.SetValue("qa")
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if got := m.valueOf("ENV"); got != "qa2" {
		t.Fatalf("typed ENV = %q, want qa2", got)
	}
	// From a non-choice value, up jumps to the last choice.
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.valueOf("ENV"); got != "dev" {
		t.Fatalf("after up from free text, ENV = %q, want dev", got)
	}
}

func TestCtrlN_ResetsDeclaredFieldsToDefaults(t *testing.T) {
	m := newDefsModel(t, store.History{})
	m.addField("EXTRA", "x", false)
	m.fieldByName("ENV").Input.SetValue("dev")
	m.fieldByName("VSS").Input.SetValue("changed")
	m.focus = FocusInput
	m.focusField(0)

	m.handleInputKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.hasField("EXTRA") {
		t.Fatal("ad-hoc field EXTRA should be dropped by Ctrl+N")
	}
	if got := m.valueOf("ENV"); got != "prod" {
		t.Fatalf("ENV = %q, want default prod", got)
	}
	if got := m.valueOf("VSS"); got != "v1" {
		t.Fatalf("VSS = %q, want default v1", got)
	}
}

func TestCtrlD_NoOpOnOptionalField(t *testing.T) {
	m := newDefsModel(t, store.History{})
	m.focus = FocusInput
	m.focusField(m.fieldIndex("TENANT"))
	m.handleInputKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !m.hasField("TENANT") {
		t.Fatal("Ctrl+D must not delete a declared optional field")
	}
}

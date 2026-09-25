// Package ui implements the bubbletea model/view for tuirunner: the
// three-area TUI (input/nav/preview), profile switching, favorites,
// filtering and the session variable pool.
package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tuirunner/internal/library"
	"tuirunner/internal/store"
)

// Focus identifies which area currently receives non-modal key input.
type Focus int

const (
	FocusNav Focus = iota
	FocusInput
)

const fieldInputWidth = 40

// Field is one entry in the session variable pool / input-area form.
type Field struct {
	Name     string
	MustHave bool
	Input    textinput.Model
}

// ListItem is one row of a rendered (favorite-pinned, filtered) script
// list.
type ListItem struct {
	Entry    library.Entry
	Favorite bool
}

// Model is the top-level bubbletea model.
type Model struct {
	root    string
	profile *store.Profile

	fields []Field

	focus     Focus
	activeTab int
	// selection remembers, per library key, the name of the last
	// selected entry so it survives tab switches and filtering.
	selection map[string]string

	filterMode  bool
	filterInput textinput.Model

	inputIndex int

	addVarMode  bool
	addVarInput textinput.Model

	profileSwitcherOpen  bool
	profileNames         []string
	profileSwitcherIndex int

	confirmAddVarsOpen  bool
	confirmAddVarsNames []string

	// historyRuns is the capped (store.MaxHistoryRuns) log of past
	// executions, most recent first, shown by the Ctrl+H overlay and
	// persisted back into history.yaml on every save.
	historyRuns  []store.HistoryEntry
	historyOpen  bool
	historyIndex int

	// escQuitArmed is set by a first Esc press (in nav or input focus,
	// outside any submode that already gives Esc its own meaning) and
	// quits on the very next Esc; any other key disarms it.
	escQuitArmed bool

	width, height int

	execCommand string
	saveErr     error

	// lastRenderLines is how many terminal lines the most recent View()
	// call occupied, so main can erase exactly that much of the inline
	// (non-altscreen) terminal output once the program quits.
	lastRenderLines int
}

// LastRenderLines returns how many terminal lines the final rendered frame
// occupied, for erasing it after the program exits.
func (m *Model) LastRenderLines() int { return m.lastRenderLines }

// NewModel builds the initial model for profile, seeded from hist (the
// persisted last-run state) and positional (CLI positional args, which
// always override history for the corresponding must-have slots).
func NewModel(root string, profile *store.Profile, hist store.History, positional []string) *Model {
	m := &Model{
		root:        root,
		profile:     profile,
		selection:   map[string]string{},
		focus:       FocusNav,
		historyRuns: append([]store.HistoryEntry(nil), hist.Runs...),
	}

	m.filterInput = textinput.New()
	m.filterInput.Prompt = "/"
	m.filterInput.Width = fieldInputWidth

	m.addVarInput = textinput.New()
	m.addVarInput.Prompt = "name: "
	m.addVarInput.Width = fieldInputWidth

	for i, name := range profile.MustHave {
		val := ""
		if hv, ok := hist.Vars.Get(name); ok {
			val = hv
		}
		if i < len(positional) {
			val = positional[i]
		}
		m.addField(name, val, true)
	}
	for _, pair := range hist.Vars {
		if m.hasField(pair.Name) {
			continue
		}
		m.addField(pair.Name, pair.Value, false)
	}

	if hist.Profile == profile.Name && hist.Library != "" {
		for i, lib := range profile.Libraries {
			if lib.Key == hist.Library {
				m.activeTab = i
				if hist.Script != "" {
					m.selection[lib.Key] = hist.Script
				}
				break
			}
		}
	}

	m.ensureCurrentSelectionValid()
	return m
}

// ExecCommand returns the flattened+substituted shell command to run after
// the bubbletea program exits, or "" if the program should just quit.
func (m *Model) ExecCommand() string { return m.execCommand }

// SaveErr returns any error encountered the last time history was
// persisted.
func (m *Model) SaveErr() error { return m.saveErr }

// ---- field / variable pool helpers ----

func (m *Model) fieldIndex(name string) int {
	for i, f := range m.fields {
		if f.Name == name {
			return i
		}
	}
	return -1
}

func (m *Model) hasField(name string) bool { return m.fieldIndex(name) >= 0 }

func (m *Model) valueOf(name string) string {
	if i := m.fieldIndex(name); i >= 0 {
		return m.fields[i].Input.Value()
	}
	return ""
}

func (m *Model) addField(name, value string, mustHave bool) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Width = fieldInputWidth
	ti.SetValue(value)
	m.fields = append(m.fields, Field{Name: name, MustHave: mustHave, Input: ti})
}

func (m *Model) valuesMap() map[string]string {
	out := make(map[string]string, len(m.fields))
	for _, f := range m.fields {
		out[f.Name] = f.Input.Value()
	}
	return out
}

// missingFieldsFor returns the placeholders entry references that have no
// corresponding field in the pool yet (as opposed to a field that exists
// but is merely empty).
func (m *Model) missingFieldsFor(entry *library.Entry) []string {
	if entry == nil {
		return nil
	}
	var out []string
	for _, ph := range entry.Placeholders {
		if !m.hasField(ph) {
			out = append(out, ph)
		}
	}
	return out
}

// focusField focuses exactly field i of m.fields (blurring the rest).
func (m *Model) focusField(i int) {
	for idx := range m.fields {
		if idx == i {
			m.fields[idx].Input.Focus()
		} else {
			m.fields[idx].Input.Blur()
		}
	}
	m.inputIndex = i
}

func (m *Model) blurAllFields() {
	for idx := range m.fields {
		m.fields[idx].Input.Blur()
	}
}

// ensureInputFocusValid clamps inputIndex into range and focuses it. Used
// whenever focus moves onto the input area.
func (m *Model) ensureInputFocusValid() {
	if len(m.fields) == 0 {
		m.inputIndex = -1
		return
	}
	if m.inputIndex < 0 || m.inputIndex >= len(m.fields) {
		m.inputIndex = 0
	}
	m.focusField(m.inputIndex)
}

// ---- nav / library helpers ----

func (m *Model) currentLibrary() (library.Library, bool) {
	if m.activeTab < 0 || m.activeTab >= len(m.profile.Libraries) {
		return library.Library{}, false
	}
	return m.profile.Libraries[m.activeTab], true
}

// currentItems returns the currently visible (favorite-pinned, filtered)
// script list for the active tab.
func (m *Model) currentItems() []ListItem {
	lib, ok := m.currentLibrary()
	if !ok {
		return nil
	}
	return visibleEntries(lib, m.profile.Favorites, m.filterInput.Value())
}

func visibleEntries(lib library.Library, fav store.FavoritesFile, filter string) []ListItem {
	favNames := fav.Favorites[lib.Key]
	favSet := make(map[string]bool, len(favNames))
	for _, n := range favNames {
		favSet[n] = true
	}
	lf := strings.ToLower(filter)
	matches := func(name string) bool {
		return filter == "" || strings.Contains(strings.ToLower(name), lf)
	}

	var items []ListItem
	for _, n := range favNames {
		if e, ok := lib.FindEntry(n); ok && matches(e.Name) {
			items = append(items, ListItem{Entry: e, Favorite: true})
		}
	}
	for _, e := range lib.Entries {
		if favSet[e.Name] {
			continue
		}
		if matches(e.Name) {
			items = append(items, ListItem{Entry: e, Favorite: false})
		}
	}
	return items
}

func (m *Model) currentIndex(items []ListItem) int {
	lib, ok := m.currentLibrary()
	if !ok || len(items) == 0 {
		return -1
	}
	name := m.selection[lib.Key]
	for i, it := range items {
		if it.Entry.Name == name {
			return i
		}
	}
	return 0
}

func (m *Model) selectedEntry() *library.Entry {
	items := m.currentItems()
	idx := m.currentIndex(items)
	if idx < 0 || idx >= len(items) {
		return nil
	}
	e := items[idx].Entry
	return &e
}

// ensureCurrentSelectionValid snaps the remembered selection for the
// active tab onto a real, currently-visible item.
func (m *Model) ensureCurrentSelectionValid() {
	lib, ok := m.currentLibrary()
	if !ok {
		return
	}
	items := m.currentItems()
	if len(items) == 0 {
		return
	}
	name := m.selection[lib.Key]
	for _, it := range items {
		if it.Entry.Name == name {
			return
		}
	}
	m.selection[lib.Key] = items[0].Entry.Name
}

func (m *Model) moveSelection(delta int) {
	lib, ok := m.currentLibrary()
	if !ok {
		return
	}
	items := m.currentItems()
	if len(items) == 0 {
		return
	}
	idx := m.currentIndex(items)
	idx = clamp(idx+delta, 0, len(items)-1)
	m.selection[lib.Key] = items[idx].Entry.Name
}

func (m *Model) switchTab(delta int) {
	n := len(m.profile.Libraries)
	if n == 0 {
		return
	}
	m.activeTab = ((m.activeTab+delta)%n + n) % n
	m.filterMode = false
	m.filterInput.SetValue("")
	m.filterInput.Blur()
	m.ensureCurrentSelectionValid()
}

func (m *Model) syncSelectionAfterFilterChange() {
	m.ensureCurrentSelectionValid()
}

func (m *Model) toggleFavorite() {
	lib, ok := m.currentLibrary()
	if !ok {
		return
	}
	entry := m.selectedEntry()
	if entry == nil {
		return
	}
	m.profile.Favorites.Toggle(lib.Key, entry.Name)
	m.saveErr = m.profile.SaveFavorites()
}

// ---- profile switching ----

func (m *Model) openProfileSwitcher() {
	names, err := listProfiles(m.root)
	if err != nil {
		names = nil
	}
	m.profileNames = names
	m.profileSwitcherIndex = 0
	for i, n := range names {
		if n == m.profile.Name {
			m.profileSwitcherIndex = i
			break
		}
	}
	m.profileSwitcherOpen = true
}

// ---- run history ----

func (m *Model) openHistorySwitcher() {
	m.historyIndex = 0
	m.historyOpen = true
}

func (m *Model) switchProfile(name string) {
	p, err := store.LoadProfile(m.root, name)
	if err != nil {
		// Can't switch (e.g. raced with an external deletion); keep the
		// current profile active.
		return
	}
	m.profile = p
	m.fields = nil
	for _, mh := range p.MustHave {
		m.addField(mh, "", true)
	}
	m.activeTab = 0
	m.selection = map[string]string{}
	m.filterMode = false
	m.filterInput.SetValue("")
	m.focus = FocusNav
	m.blurAllFields()
	m.inputIndex = -1
	m.ensureCurrentSelectionValid()
}

// ---- validation / execution ----

func (m *Model) tryExecute() tea.Cmd {
	entry := m.selectedEntry()
	if entry == nil {
		return nil
	}
	if missing := m.missingFieldsFor(entry); len(missing) > 0 {
		m.confirmAddVarsNames = missing
		m.confirmAddVarsOpen = true
		return nil
	}
	for _, name := range m.profile.MustHave {
		if m.valueOf(name) == "" {
			return nil
		}
	}
	for _, ph := range entry.Placeholders {
		if m.valueOf(ph) == "" {
			return nil
		}
	}
	m.execCommand = library.Substitute(entry.FlatCommand, m.valuesMap())
	m.recordRun()
	m.saveHistory()
	return tea.Quit
}

// recordRun snapshots the current profile/library/script/vars as a new
// most-recent entry in the Ctrl+H history log, capped at
// store.MaxHistoryRuns.
func (m *Model) recordRun() {
	libKey, script := m.currentLibKeyAndScript()
	entry := store.HistoryEntry{
		Profile: m.profile.Name,
		Library: libKey,
		Script:  script,
		Vars:    m.fieldsAsVarList(),
		When:    time.Now(),
	}
	m.historyRuns = append([]store.HistoryEntry{entry}, m.historyRuns...)
	if len(m.historyRuns) > store.MaxHistoryRuns {
		m.historyRuns = m.historyRuns[:store.MaxHistoryRuns]
	}
}

// restoreRun loads e's profile (switching to it if needed), library tab,
// script selection and variable values into the current session, as if
// you'd navigated there yourself. Does not execute anything.
func (m *Model) restoreRun(e store.HistoryEntry) {
	if e.Profile != m.profile.Name {
		p, err := store.LoadProfile(m.root, e.Profile)
		if err != nil {
			return // profile no longer exists; leave the session as-is
		}
		m.profile = p
	}

	m.fields = nil
	for _, name := range m.profile.MustHave {
		val := ""
		if v, ok := e.Vars.Get(name); ok {
			val = v
		}
		m.addField(name, val, true)
	}
	for _, pair := range e.Vars {
		if m.hasField(pair.Name) {
			continue
		}
		m.addField(pair.Name, pair.Value, false)
	}

	m.activeTab = 0
	m.selection = map[string]string{}
	for i, lib := range m.profile.Libraries {
		if lib.Key == e.Library {
			m.activeTab = i
			if e.Script != "" {
				m.selection[lib.Key] = e.Script
			}
			break
		}
	}

	m.filterMode = false
	m.filterInput.SetValue("")
	m.focus = FocusNav
	m.blurAllFields()
	m.inputIndex = -1
	m.ensureCurrentSelectionValid()
}

// confirmAddMissingVars adds the pending confirmAddVarsNames as new empty
// ad-hoc fields and moves focus to the first one so it can be filled in
// immediately. Called after the user accepts the "add these variables?"
// prompt raised by tryExecute.
func (m *Model) confirmAddMissingVars() {
	names := m.confirmAddVarsNames
	m.confirmAddVarsNames = nil
	m.confirmAddVarsOpen = false
	for _, name := range names {
		if !m.hasField(name) {
			m.addField(name, "", false)
		}
	}
	if len(names) > 0 {
		m.focus = FocusInput
		if idx := m.fieldIndex(names[0]); idx >= 0 {
			m.focusField(idx)
		}
	}
}

// cancelAddMissingVars declines the pending "add these variables?" prompt:
// nothing is added, nothing runs.
func (m *Model) cancelAddMissingVars() {
	m.confirmAddVarsNames = nil
	m.confirmAddVarsOpen = false
}

func (m *Model) quitAndSave() tea.Cmd {
	m.execCommand = ""
	m.saveHistory()
	return tea.Quit
}

// currentLibKeyAndScript returns the active library's key and the
// currently selected script's name (either may be "" if nothing is
// selectable right now).
func (m *Model) currentLibKeyAndScript() (libKey, script string) {
	if lib, ok := m.currentLibrary(); ok {
		libKey = lib.Key
	}
	if e := m.selectedEntry(); e != nil {
		script = e.Name
	}
	return libKey, script
}

// fieldsAsVarList snapshots the current session variable pool as a
// store.VarList, preserving field order.
func (m *Model) fieldsAsVarList() store.VarList {
	vars := make(store.VarList, 0, len(m.fields))
	for _, f := range m.fields {
		vars = append(vars, store.VarPair{Name: f.Name, Value: f.Input.Value()})
	}
	return vars
}

func (m *Model) buildHistory() store.History {
	libKey, script := m.currentLibKeyAndScript()
	return store.History{
		Profile: m.profile.Name,
		Library: libKey,
		Script:  script,
		Vars:    m.fieldsAsVarList(),
		Runs:    m.historyRuns,
	}
}

func (m *Model) saveHistory() {
	m.saveErr = m.buildHistory().Save(m.root)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// listProfiles is a thin wrapper kept local so ui doesn't need to import
// store's ListProfiles name twice in call sites.
func listProfiles(root string) ([]string, error) {
	return store.ListProfiles(root)
}

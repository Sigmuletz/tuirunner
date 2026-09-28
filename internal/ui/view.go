package ui

import (
	"fmt"
	"strings"

	"tuirunner/internal/library"
)

const maxPreviewCmdLines = 12

// maxContentWidth caps the box width so it doesn't stretch edge-to-edge on
// wide terminals. maxNavListRows caps visible script rows so the box grows
// with the list (short lists = short box) instead of always filling the
// terminal height; longer lists still scroll within this cap.
const (
	maxContentWidth = 100
	maxNavListRows  = 12
)

// View satisfies tea.Model.
func (m *Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	if width > maxContentWidth {
		width = maxContentWidth
	}
	innerWidth := width - 2
	if innerWidth < 20 {
		innerWidth = 20
	}

	if m.profileSwitcherOpen {
		height := len(m.profileNames) + 2
		if height < 3 {
			height = 3
		}
		return m.recordRendered(m.renderSwitcherOverlay(innerWidth, height))
	}

	if m.historyOpen {
		height := len(m.historyRuns) + 2
		if height < 3 {
			height = 3
		}
		return m.recordRendered(m.renderHistoryOverlay(innerWidth, height))
	}

	inputLines := m.renderInputLines(innerWidth)
	previewLines := m.renderPreviewLines(innerWidth)

	listRows := len(m.currentItems())
	if listRows < 1 {
		listRows = 1
	}
	if listRows > maxNavListRows {
		listRows = maxNavListRows
	}
	navHeight := listRows + 1 // +1 for the tab row
	if m.filterMode {
		navHeight++
	}
	navLines := m.renderNavLines(innerWidth, navHeight)

	var all []string
	all = append(all, inputLines...)
	all = append(all, dividerLine(innerWidth))
	all = append(all, navLines...)
	all = append(all, dividerLine(innerWidth))
	all = append(all, previewLines...)

	content := strings.Join(all, "\n")
	return m.recordRendered(outerBorderStyle.Width(innerWidth).Render(content))
}

// recordRendered stashes how many terminal lines rendered occupies so that,
// once the program quits, main can erase exactly that many lines from an
// inline (non-altscreen) terminal instead of leaving the last frame behind.
func (m *Model) recordRendered(rendered string) string {
	m.lastRenderLines = strings.Count(rendered, "\n") + 1
	return rendered
}

func dividerLine(width int) string {
	if width < 0 {
		width = 0
	}
	return dividerStyle.Width(width).Render(strings.Repeat("─", width))
}

// ---- input area ----

func (m *Model) renderInputLines(width int) []string {
	var lines []string
	header := headingStyle.Render("VARIABLES") + "  " +
		dimStyle.Render("(Tab: focus fields | ←/→ pick dropdown | ↑/↓ choices | Ctrl+R clear | Ctrl+N new | Ctrl+A add | Ctrl+D delete)")
	lines = append(lines, padLine(header, width))

	if len(m.fields) == 0 {
		lines = append(lines, padLine(dimStyle.Render("  (no variables)"), width))
	}

	// relevant marks fields referenced by the currently selected script,
	// recomputed fresh every render so the highlight tracks selection
	// live instead of sticking to whatever was last picked.
	relevant := map[string]bool{}
	if e := m.selectedEntry(); e != nil {
		for _, ph := range e.Placeholders {
			relevant[ph] = true
		}
	}

	labelWidth := 0
	for _, f := range m.fields {
		n := len(f.Name) + 1 // +1 for possible "*"
		if n > labelWidth {
			labelWidth = n
		}
	}

	for i, f := range m.fields {
		label := f.Name
		if f.MustHave {
			label += "*"
		}
		if f.Kind == FieldComputed {
			label += " (auto)"
		}
		label += ":"
		for len(label) < labelWidth+1 {
			label += " "
		}

		focused := m.focus == FocusInput && i == m.inputIndex
		var labelRendered string
		switch {
		case focused:
			labelRendered = highlightStyle.Render(label)
		case relevant[f.Name]:
			labelRendered = relevantStyle.Render(label)
		case f.MustHave:
			labelRendered = lineStyle.Bold(true).Render(label)
		default:
			labelRendered = lineStyle.Render(label)
		}

		var valueRendered string
		switch f.Kind {
		case FieldDropdown:
			if len(f.Options) == 0 {
				valueRendered = dimStyle.Render(fmt.Sprintf("(fill %s_LIST first)", f.Name))
			} else {
				valueRendered = dimStyle.Render("◂ ") + f.Input.View() +
					dimStyle.Render(fmt.Sprintf(" ▸  (%d/%d)", f.SelectedIndex+1, len(f.Options)))
			}
		case FieldComputed:
			valueRendered = dimStyle.Render(f.Input.Value())
		default:
			valueRendered = f.Input.View()
			if len(f.Choices) > 0 {
				valueRendered += "  " + renderChoices(f.Choices, f.Input.Value())
			}
		}

		row := "  " + labelRendered + " " + valueRendered
		lines = append(lines, padLine(row, width))
	}

	if m.addVarMode {
		row := "  " + highlightStyle.Render("add variable name:") + " " + m.addVarInput.View()
		lines = append(lines, padLine(row, width))
	}

	return lines
}

// renderChoices shows a field's premade choices as a dim "a | b | c"
// hint, with the one matching the current value highlighted.
func renderChoices(choices []string, current string) string {
	parts := make([]string, len(choices))
	for i, c := range choices {
		if c == current {
			parts[i] = lineStyle.Bold(true).Render(c)
		} else {
			parts[i] = dimStyle.Render(c)
		}
	}
	return dimStyle.Render("[") + strings.Join(parts, dimStyle.Render(" | ")) + dimStyle.Render("]")
}

// ---- nav area ----

func (m *Model) renderNavLines(width, height int) []string {
	var lines []string

	var tabParts []string
	for i, lib := range m.profile.Libraries {
		if i == m.activeTab {
			tabParts = append(tabParts, highlightStyle.Render("["+lib.Label+"]"))
		} else {
			tabParts = append(tabParts, dimStyle.Render(" "+lib.Label+" "))
		}
	}
	tabsLine := strings.Join(tabParts, " ")
	if m.escQuitArmed {
		tabsLine += "  " + dimStyle.Render("(press Esc again to quit)")
	}
	lines = append(lines, padLine(tabsLine, width))
	used := 1

	if m.filterMode {
		lines = append(lines, padLine("  "+highlightStyle.Render("/")+m.filterInput.View(), width))
		used++
	}

	listHeight := height - used
	if listHeight < 1 {
		listHeight = 1
	}

	items := m.currentItems()
	idx := m.currentIndex(items)

	if len(items) == 0 {
		lines = append(lines, padLine(dimStyle.Render("  (no scripts)"), width))
	} else {
		start := 0
		if idx >= listHeight {
			start = idx - listHeight + 1
		}
		end := start + listHeight
		if end > len(items) {
			end = len(items)
			start = end - listHeight
			if start < 0 {
				start = 0
			}
		}
		for i := start; i < end; i++ {
			it := items[i]
			star := " "
			if it.Favorite {
				star = "★"
			}
			marker := "  "
			starStyled := starStyle.Render(star)
			nameStyled := lineStyle.Render(it.Entry.Name)
			if i == idx {
				marker = highlightStyle.Render("▸ ")
				nameStyled = highlightStyle.Render(it.Entry.Name)
				if it.Favorite {
					starStyled = highlightStyle.Render(star)
				}
			}
			lines = append(lines, padLine(marker+starStyled+" "+nameStyled, width))
		}
	}

	for len(lines) < height {
		lines = append(lines, padLine("", width))
	}
	return lines[:height]
}

// ---- preview area ----

func (m *Model) renderPreviewLines(width int) []string {
	if m.confirmAddVarsOpen {
		return m.renderConfirmAddVarsLines(width)
	}

	var lines []string
	entry := m.selectedEntry()

	if entry == nil {
		lines = append(lines, padLine(headingStyle.Render("PREVIEW"), width))
		lines = append(lines, padLine(dimStyle.Render("  (no script selected)"), width))
		return lines
	}

	lines = append(lines, padLine(headingStyle.Render("PREVIEW")+"  "+dimStyle.Render(entry.Name), width))

	substituted := library.SubstitutePreview(entry.RawCommand, m.valuesMap())
	cmdLines := strings.Split(substituted, "\n")
	if len(cmdLines) > maxPreviewCmdLines {
		cmdLines = append(cmdLines[:maxPreviewCmdLines], "…")
	}
	for _, cl := range cmdLines {
		lines = append(lines, padLine("  "+lineStyle.Render(cl), width))
	}

	if missing := m.missingRequired(entry); len(missing) > 0 {
		lines = append(lines, padLine(dimStyle.Render("  blocked - missing: "+strings.Join(missing, ", ")), width))
	} else {
		lines = append(lines, padLine(highlightStyle.Render("  ready - press Enter to run"), width))
	}
	return lines
}

// missingRequired returns the ordered, deduplicated list of variable
// names that must be filled before entry can be executed (profile
// must-haves, then any placeholder the command itself references) but
// currently are not.
func (m *Model) missingRequired(entry *library.Entry) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if seen[name] {
			return
		}
		if m.valueOf(name) == "" {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, n := range m.profile.MustHave {
		add(n)
	}
	for _, ph := range entry.Placeholders {
		add(ph)
	}
	return out
}

// renderConfirmAddVarsLines replaces the preview area's normal content
// while tryExecute's "add these missing variables?" prompt is pending.
func (m *Model) renderConfirmAddVarsLines(width int) []string {
	var lines []string
	lines = append(lines, padLine(headingStyle.Render("NEW VARIABLES NEEDED"), width))
	lines = append(lines, padLine("  "+relevantStyle.Render(strings.Join(m.confirmAddVarsNames, ", ")), width))
	lines = append(lines, padLine(highlightStyle.Render("  add them? [y] yes   [any other key] no"), width))
	return lines
}

// ---- profile switcher overlay ----

func (m *Model) renderSwitcherOverlay(width, height int) string {
	var lines []string
	lines = append(lines, padLine(headingStyle.Render("SWITCH PROFILE")+"  "+
		dimStyle.Render("(up/down choose, enter select, esc cancel)"), width))
	lines = append(lines, padLine("", width))

	if len(m.profileNames) == 0 {
		lines = append(lines, padLine(dimStyle.Render("  (no profiles found)"), width))
	}
	for i, name := range m.profileNames {
		marker := "  "
		styled := lineStyle.Render(name)
		if i == m.profileSwitcherIndex {
			marker = highlightStyle.Render("▸ ")
			styled = highlightStyle.Render(name)
		}
		if name == m.profile.Name {
			styled += dimStyle.Render("  (current)")
		}
		lines = append(lines, padLine(marker+styled, width))
	}

	for len(lines) < height {
		lines = append(lines, padLine("", width))
	}
	content := strings.Join(lines[:height], "\n")
	return outerBorderStyle.Width(width).Height(height).Render(content)
}

// ---- run history overlay ----

func (m *Model) renderHistoryOverlay(width, height int) string {
	var lines []string
	lines = append(lines, padLine(headingStyle.Render("RUN HISTORY")+"  "+
		dimStyle.Render("(up/down choose, enter restore, esc cancel)"), width))
	lines = append(lines, padLine("", width))

	if len(m.historyRuns) == 0 {
		lines = append(lines, padLine(dimStyle.Render("  (no runs yet)"), width))
	}
	for i, run := range m.historyRuns {
		libLabel := strings.TrimSuffix(run.Library, "_library")
		text := run.Profile + "/" + libLabel + " · " + run.Script +
			"   " + run.When.Local().Format("2006-01-02 15:04:05")
		marker := "  "
		styled := lineStyle.Render(text)
		if i == m.historyIndex {
			marker = highlightStyle.Render("▸ ")
			styled = highlightStyle.Render(text)
		}
		lines = append(lines, padLine(marker+styled, width))
	}

	for len(lines) < height {
		lines = append(lines, padLine("", width))
	}
	content := strings.Join(lines[:height], "\n")
	return outerBorderStyle.Width(width).Height(height).Render(content)
}

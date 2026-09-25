// Command tuirunner is an interactive terminal UI for browsing
// pipe-delimited "library" files of shell commands, filling in {{...}}
// placeholders from a persistent per-run variable pool, and executing the
// selected command directly against the terminal.
//
// It always opens the interactive TUI; there is no non-interactive CLI
// mode.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"tuirunner/internal/store"
	"tuirunner/internal/ui"
)

func main() {
	profileFlag, positional := parseArgs(os.Args[1:])

	root, err := store.ResolveRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tuirunner: could not resolve installation directory: %v\n", err)
		os.Exit(1)
	}

	hist, err := store.LoadHistory(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tuirunner: could not read history.yaml: %v\n", err)
		os.Exit(1)
	}

	profileName := profileFlag
	if profileName == "" {
		profileName = hist.Profile
	}
	if profileName == "" {
		profileName = "default"
	}

	profile, err := store.LoadProfile(root, profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tuirunner: %v\n", err)
		os.Exit(1)
	}

	model := ui.NewModel(root, profile, hist, positional)

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tuirunner: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(*ui.Model)
	if !ok {
		os.Exit(1)
	}
	clearRenderedBox(m.LastRenderLines())
	if saveErr := m.SaveErr(); saveErr != nil {
		fmt.Fprintf(os.Stderr, "tuirunner: warning: failed to save history.yaml: %v\n", saveErr)
	}

	cmd := m.ExecCommand()
	if cmd == "" {
		os.Exit(0)
	}
	fmt.Println("$", cmd)
	runCommand(cmd)
}

// clearRenderedBox erases the last rendered frame from an inline
// (non-altscreen) terminal: bubbletea leaves the cursor immediately below
// whatever it last drew, so moving up `lines` rows and clearing from there
// to the end of the screen removes the box entirely instead of leaving it
// behind in the scrollback once the program quits.
func clearRenderedBox(lines int) {
	if lines <= 0 {
		return
	}
	fmt.Printf("\x1b[%dA\x1b[0J", lines)
}

// parseArgs splits the raw CLI args into an optional --profile value and
// the remaining positional arguments (which map, in order, onto the
// active profile's vars.yaml must_have list).
func parseArgs(args []string) (profile string, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--profile":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "tuirunner: --profile requires a value")
				os.Exit(1)
			}
			profile = args[i+1]
			i++
		case strings.HasPrefix(a, "--profile="):
			profile = strings.TrimPrefix(a, "--profile=")
		default:
			positional = append(positional, a)
		}
	}
	return profile, positional
}

// runCommand quits the TUI and executes cmd via "sh -c", attached
// directly to the terminal's stdin/stdout/stderr, with no return to the
// picker. syscall.Exec (which replaces the current process image) is
// used when available; os/exec with inherited stdio is a fallback for
// platforms/situations where exec(2) isn't viable.
func runCommand(cmd string) {
	argv := []string{"/bin/sh", "-c", cmd}
	err := syscall.Exec(argv[0], argv, os.Environ())
	if err == nil {
		return // unreachable: a successful Exec never returns
	}

	c := exec.Command("/bin/sh", "-c", cmd)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	runErr := c.Run()
	if runErr == nil {
		os.Exit(0)
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintf(os.Stderr, "tuirunner: failed to run command: %v\n", runErr)
	os.Exit(1)
}

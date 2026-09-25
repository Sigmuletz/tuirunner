package store

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MaxHistoryRuns is how many past executions History.Runs keeps, most
// recent first.
const MaxHistoryRuns = 10

// HistoryEntry is a single past execution: what was run and with which
// variable values, for the Ctrl+H history list.
type HistoryEntry struct {
	Profile string    `yaml:"profile"`
	Library string    `yaml:"library"`
	Script  string    `yaml:"script"`
	Vars    VarList   `yaml:"vars"`
	When    time.Time `yaml:"when"`
}

// History is the global (not per-profile) run state persisted to
// <root>/history.yaml: the last session (for restoring on a
// parameterless launch) plus a capped log of past executions (for the
// Ctrl+H history list).
type History struct {
	Profile string  `yaml:"profile"`
	Library string  `yaml:"library"`
	Script  string  `yaml:"script"`
	Vars    VarList `yaml:"vars"`

	Runs []HistoryEntry `yaml:"runs"`
}

func historyPath(root string) string {
	return filepath.Join(root, "history.yaml")
}

// LoadHistory reads <root>/history.yaml. If it doesn't exist, a zero-value
// History is returned with no error (Profile will be "").
func LoadHistory(root string) (History, error) {
	var h History
	data, err := os.ReadFile(historyPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return h, err
	}
	if err := yaml.Unmarshal(data, &h); err != nil {
		return h, err
	}
	return h, nil
}

// Save writes h to <root>/history.yaml.
func (h History) Save(root string) error {
	data, err := yaml.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(historyPath(root), data, 0o644)
}

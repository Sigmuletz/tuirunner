// Package store manages tuirunner's on-disk state: the root directory
// layout, per-profile libraries/favorites/vars, and the global history
// file.
package store

import (
	"os"
	"path/filepath"
	"sort"
)

// ResolveRoot returns the directory containing the running binary's real
// (symlink-resolved) executable path. Profiles and history.yaml live
// directly inside this directory.
func ResolveRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Fall back to the unresolved path rather than failing outright
		// (e.g. a dangling intermediate symlink component).
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// IsProfileDir reports whether dir (a direct child of root) looks like a
// profile: it exists and contains a "libraries" subdirectory.
func IsProfileDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "libraries"))
	return err == nil && info.IsDir()
}

// ListProfiles returns the names of every profile directory found
// directly under root, sorted alphabetically.
func ListProfiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if IsProfileDir(filepath.Join(root, e.Name())) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

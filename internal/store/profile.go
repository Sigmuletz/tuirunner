package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"tuirunner/internal/library"
)

// defaultMustHave is the must-have variable set that ships with the
// "default" profile the first time tuirunner creates its vars.yaml.
var defaultMustHave = []VarDef{{Name: "VSS"}, {Name: "KVNR_LIST"}}

// VarsFile is the per-profile vars.yaml contents. must_have variables
// must be filled before anything runs; optional ones are always shown in
// the form too, but only need a value when a script actually uses them.
// Either kind may carry a default value and premade choices (see VarDef).
type VarsFile struct {
	MustHave []VarDef `yaml:"must_have"`
	Optional []VarDef `yaml:"optional,omitempty"`
}

// FavoritesFile is the per-profile favorites.yaml contents: library
// filename -> favorited entry names, in the order they were favorited.
type FavoritesFile struct {
	Favorites map[string][]string `yaml:"favorites"`
}

// IsFavorite reports whether entryName is favorited within library libKey.
func (f FavoritesFile) IsFavorite(libKey, entryName string) bool {
	for _, n := range f.Favorites[libKey] {
		if n == entryName {
			return true
		}
	}
	return false
}

// Toggle flips the favorite state of entryName within libKey and returns
// the new state (true = now favorited).
func (f *FavoritesFile) Toggle(libKey, entryName string) bool {
	if f.Favorites == nil {
		f.Favorites = map[string][]string{}
	}
	list := f.Favorites[libKey]
	for i, n := range list {
		if n == entryName {
			f.Favorites[libKey] = append(list[:i], list[i+1:]...)
			return false
		}
	}
	f.Favorites[libKey] = append(list, entryName)
	return true
}

// Profile is a fully loaded profile: its libraries, favorites and
// declared variable configuration.
type Profile struct {
	Name      string
	Dir       string
	Libraries []library.Library
	Favorites FavoritesFile
	MustHave  []string
	// Optional lists the names of vars.yaml's optional variables, in
	// file order.
	Optional []string
	// Defs holds the default/choices for every declared variable (must
	// have or optional), keyed by name.
	Defs map[string]VarDef
}

// Def returns the vars.yaml definition for name (the zero VarDef, with no
// default or choices, if it isn't declared).
func (p *Profile) Def(name string) VarDef {
	if d, ok := p.Defs[name]; ok {
		return d
	}
	return VarDef{Name: name}
}

func profileDir(root, name string) string {
	return filepath.Join(root, name)
}

// LoadProfile loads the named profile from root. The profile directory
// must already exist (tuirunner never auto-creates or migrates profile
// folders) — a missing directory is a hard error. Missing favorites.yaml
// or vars.yaml *within* an existing profile directory are created with
// sensible empty defaults (the "default" profile additionally seeds
// vars.yaml with must_have: [VSS, KVNR_LIST] per spec).
func LoadProfile(root, name string) (*Profile, error) {
	dir := profileDir(root, name)
	if !IsProfileDir(dir) {
		return nil, fmt.Errorf("profile %q not found (expected a %q directory with a libraries/ subdirectory)", name, dir)
	}

	libs, err := library.LoadDir(filepath.Join(dir, "libraries"))
	if err != nil {
		return nil, fmt.Errorf("loading libraries for profile %q: %w", name, err)
	}

	vf, err := loadOrCreateVars(dir, name)
	if err != nil {
		return nil, err
	}

	ff, err := loadOrCreateFavorites(dir)
	if err != nil {
		return nil, err
	}

	p := &Profile{
		Name:      name,
		Dir:       dir,
		Libraries: libs,
		Favorites: ff,
		Defs:      map[string]VarDef{},
	}
	for _, d := range vf.MustHave {
		p.MustHave = append(p.MustHave, d.Name)
		p.Defs[d.Name] = d
	}
	for _, d := range vf.Optional {
		if _, dup := p.Defs[d.Name]; dup {
			continue // already declared as must_have (or listed twice)
		}
		p.Optional = append(p.Optional, d.Name)
		p.Defs[d.Name] = d
	}
	return p, nil
}

func varsPath(dir string) string      { return filepath.Join(dir, "vars.yaml") }
func favoritesPath(dir string) string { return filepath.Join(dir, "favorites.yaml") }

func loadOrCreateVars(dir, profileName string) (VarsFile, error) {
	path := varsPath(dir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		vf := VarsFile{}
		if profileName == "default" {
			vf.MustHave = append([]VarDef{}, defaultMustHave...)
		}
		if err := writeYAML(path, vf); err != nil {
			return vf, err
		}
		return vf, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return VarsFile{}, err
	}
	var vf VarsFile
	if err := yaml.Unmarshal(data, &vf); err != nil {
		return VarsFile{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return vf, nil
}

func loadOrCreateFavorites(dir string) (FavoritesFile, error) {
	path := favoritesPath(dir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		ff := FavoritesFile{Favorites: map[string][]string{}}
		if err := writeYAML(path, ff); err != nil {
			return ff, err
		}
		return ff, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FavoritesFile{}, err
	}
	var ff FavoritesFile
	if err := yaml.Unmarshal(data, &ff); err != nil {
		return FavoritesFile{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if ff.Favorites == nil {
		ff.Favorites = map[string][]string{}
	}
	return ff, nil
}

// SaveFavorites persists the profile's current favorites to disk.
func (p *Profile) SaveFavorites() error {
	return writeYAML(favoritesPath(p.Dir), p.Favorites)
}

func writeYAML(path string, v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FindLibrary returns the library with the given key (filename), if
// present.
func (p *Profile) FindLibrary(key string) (library.Library, bool) {
	for _, l := range p.Libraries {
		if l.Key == key {
			return l, true
		}
	}
	return library.Library{}, false
}

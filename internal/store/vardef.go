package store

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// VarDef is one variable declared in vars.yaml. In YAML it is either a
// bare name:
//
//   - VSS
//
// or a mapping that also gives it a default value and/or a list of
// premade choices:
//
//   - name: ENV
//     default: prod
//     choices: [prod, preprod, dev]
//
// Choices are suggestions only (cycled with ↑/↓ in the UI) — the field
// always stays free text, so any other value can still be typed.
type VarDef struct {
	Name    string   `yaml:"name"`
	Default string   `yaml:"default,omitempty"`
	Choices []string `yaml:"choices,omitempty"`
}

// UnmarshalYAML accepts either a plain scalar (just the name) or a
// {name, default, choices} mapping.
func (d *VarDef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*d = VarDef{Name: node.Value}
		return nil
	}
	type plain VarDef
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	if p.Name == "" {
		return fmt.Errorf("line %d: variable definition is missing a name", node.Line)
	}
	*d = VarDef(p)
	return nil
}

// MarshalYAML writes a bare name when there's nothing else to say, so
// simple vars.yaml files stay as simple as the ones people hand-write.
func (d VarDef) MarshalYAML() (interface{}, error) {
	if d.Default == "" && len(d.Choices) == 0 {
		return d.Name, nil
	}
	type plain VarDef
	return plain(d), nil
}

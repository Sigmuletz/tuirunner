package store

import "gopkg.in/yaml.v3"

// VarPair is one named session variable and its current value.
type VarPair struct {
	Name  string
	Value string
}

// VarList is an ordered set of variables. It marshals to/from YAML as an
// ordinary mapping (so history.yaml stays human-readable), but preserves
// insertion order on the Go side, which tuirunner relies on for the
// input-area field order ("must-haves first, then ad-hoc vars in the
// order they were added").
type VarList []VarPair

// Get returns the value for name and whether it was present.
func (v VarList) Get(name string) (string, bool) {
	for _, p := range v {
		if p.Name == name {
			return p.Value, true
		}
	}
	return "", false
}

// MarshalYAML implements yaml.Marshaler, emitting the pairs as a mapping
// node in insertion order.
func (v VarList) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, p := range v {
		k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Name}
		val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Value}
		node.Content = append(node.Content, k, val)
	}
	return node, nil
}

// UnmarshalYAML implements yaml.Unmarshaler, reading a mapping node while
// preserving the order keys appear in the source file.
func (v *VarList) UnmarshalYAML(node *yaml.Node) error {
	*v = nil
	if node == nil || node.Kind == 0 {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		var name, val string
		if err := node.Content[i].Decode(&name); err != nil {
			return err
		}
		if err := node.Content[i+1].Decode(&val); err != nil {
			return err
		}
		*v = append(*v, VarPair{Name: name, Value: val})
	}
	return nil
}

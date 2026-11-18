package platform

import (
	"bytes"
	"errors"
	"io"
	"reflect"

	"go.yaml.in/yaml/v3"
)

// Viper drops empty mappings before UnmarshalExact. Inspect the original YAML
// keys so an empty test/tool section cannot silently enter runtime configuration.
func validateConfigKeys(raw []byte) error {
	bad := errors.New("unknown, duplicate or invalid YAML configuration fields")
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var doc, extra yaml.Node
	if decoder.Decode(&doc) != nil || len(doc.Content) != 1 || !errors.Is(decoder.Decode(&extra), io.EOF) {
		return bad
	}
	var check func(*yaml.Node, reflect.Type) bool
	check = func(n *yaml.Node, typ reflect.Type) bool {
		if n.Anchor != "" || n.Kind == yaml.AliasNode {
			return false
		}
		if typ.Kind() == reflect.Pointer {
			if n.Tag == "!!null" {
				return true
			}
			return check(n, typ.Elem())
		}
		if typ.Kind() == reflect.Slice {
			if n.Kind != yaml.SequenceNode {
				return false
			}
			for _, item := range n.Content {
				if !check(item, typ.Elem()) {
					return false
				}
			}
			return true
		}
		if typ.Kind() != reflect.Struct {
			return true
		}
		if n.Kind != yaml.MappingNode {
			return false
		}
		fields := make(map[string]reflect.Type)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if name := f.Tag.Get("mapstructure"); name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		seen := make(map[string]bool)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			field, ok := fields[key.Value]
			if key.Tag != "!!str" || !ok || seen[key.Value] || !check(n.Content[i+1], field) {
				return false
			}
			seen[key.Value] = true
		}
		return true
	}
	if !check(doc.Content[0], reflect.TypeFor[Config]()) {
		return bad
	}
	return nil
}

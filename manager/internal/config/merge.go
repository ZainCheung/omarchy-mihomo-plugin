package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func Parse(data []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	n, ok := normalize(raw).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("YAML root must be a map")
	}
	return n, nil
}
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			x[k] = normalize(vv)
		}
		return x
	case map[any]any:
		m := map[string]any{}
		for k, vv := range x {
			m[fmt.Sprint(k)] = normalize(vv)
		}
		return m
	case []any:
		for i, vv := range x {
			x[i] = normalize(vv)
		}
		return x
	default:
		return v
	}
}
func clone(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, vv := range x {
			m[k] = clone(vv)
		}
		return m
	case []any:
		a := make([]any, len(x))
		for i, vv := range x {
			a[i] = clone(vv)
		}
		return a
	default:
		return x
	}
}

// DeepMerge applies the V1 policy: maps recurse, arrays and scalars replace,
// and null in the later map deletes the key.
func DeepMerge(base, overlay map[string]any) map[string]any {
	out := clone(base).(map[string]any)
	for k, v := range overlay {
		if v == nil {
			delete(out, k)
			continue
		}
		if om, ok := v.(map[string]any); ok {
			if bm, ok := out[k].(map[string]any); ok {
				out[k] = DeepMerge(bm, om)
			} else {
				out[k] = clone(om)
			}
			continue
		}
		out[k] = clone(v)
	}
	return out
}
func ParseOptional(data []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	return Parse(data)
}
func Marshal(m map[string]any) ([]byte, error) { return yaml.Marshal(m) }

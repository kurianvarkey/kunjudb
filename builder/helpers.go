package builder

import (
	"reflect"
	"sort"
)

// sortedKeys returns map keys in sorted order for deterministic SQL.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// structToMap converts a struct's `db`-tagged fields into a column-value map.
func structToMap(entity any) map[string]any {
	v := reflect.Indirect(reflect.ValueOf(entity))
	t := v.Type()
	out := make(map[string]any, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		out[tag] = v.Field(i).Interface()
	}
	return out
}

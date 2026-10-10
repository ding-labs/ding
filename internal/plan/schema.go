package plan

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/ding-labs/ding/internal/watch"
)

// JSONSchema describes the structural authoring contract. Compile additionally
// checks semantic combinations, durations, references, and expression grammar.
// Types are reflected to keep field names and additionalProperties in sync.
func JSONSchema() ([]byte, error) {
	definitions := map[string]any{}
	required := map[string][]string{
		"Definition": {"apiVersion", "kind", "metadata", "spec"}, "Destination": {"apiVersion", "kind", "metadata", "spec"},
		"Metadata": {"id"}, "Spec": {"source", "condition"}, "Source": {"type"}, "SecretRef": {"env"}, "Target": {"ref"}, "DestinationSpec": {"type"},
	}
	var describe func(reflect.Type) map[string]any
	describe = func(t reflect.Type) map[string]any {
		if t.Kind() == reflect.Pointer {
			return describe(t.Elem())
		}
		switch t.Kind() {
		case reflect.Struct:
			if _, ok := definitions[t.Name()]; !ok {
				props := map[string]any{}
				def := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
				definitions[t.Name()] = def
				if fields := required[t.Name()]; len(fields) > 0 {
					def["required"] = fields
				}
				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)
					name := strings.Split(f.Tag.Get("json"), ",")[0]
					if name == "" || name == "-" {
						continue
					}
					props[name] = describe(f.Type)
				}
			}
			return map[string]any{"$ref": "#/$defs/" + t.Name()}
		case reflect.Slice:
			return map[string]any{"type": "array", "items": describe(t.Elem())}
		case reflect.Map:
			return map[string]any{"type": "object", "additionalProperties": describe(t.Elem())}
		case reflect.Int, reflect.Int64:
			return map[string]any{"type": "integer"}
		case reflect.String:
			return map[string]any{"type": "string"}
		default:
			return map[string]any{"type": []string{"string", "number", "boolean", "null"}}
		}
	}
	w := describe(reflect.TypeOf(watch.Definition{}))
	d := describe(reflect.TypeOf(watch.Destination{}))
	prop := func(kind, name string) map[string]any {
		return definitions[kind].(map[string]any)["properties"].(map[string]any)[name].(map[string]any)
	}
	for typ, kind := range map[string]string{"Definition": "Watch", "Destination": "Destination"} {
		prop(typ, "apiVersion")["const"] = watch.APIVersion
		prop(typ, "kind")["const"] = kind
	}
	prop("Metadata", "id")["pattern"] = identifier.String()
	prop("SecretRef", "env")["pattern"] = envName.String()
	prop("Source", "type")["enum"] = []string{"http", "command", "push"}
	prop("DestinationSpec", "type")["enum"] = []string{"console", "desktop", "webhook", "slack", "discord"}
	prop("Condition", "operator")["enum"] = []string{"eq", "ne", "gt", "gte", "lt", "lte", "changed", "new-event"}
	prop("Policy", "trigger")["enum"] = []string{"transition", "level"}
	prop("Policy", "onUnknown")["const"] = "hold-incident"
	// value is required for comparison, including explicit JSON null.
	condition := definitions["Condition"].(map[string]any)
	condition["allOf"] = []any{map[string]any{"if": map[string]any{"required": []string{"operator"}, "properties": map[string]any{"operator": map[string]any{"enum": []string{"eq", "ne", "gt", "gte", "lt", "lte"}}}}, "then": map[string]any{"required": []string{"value"}}}}
	schema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://ding.ing/schema/watch-v1alpha1.json", "title": "Ding watch resources", "oneOf": []any{w, d}, "$defs": definitions}
	return json.MarshalIndent(schema, "", "  ")
}

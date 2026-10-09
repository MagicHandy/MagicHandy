package llm

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
)

// OpenAISchema maps our omission-based domain schemas into OpenAI strict
// schemas. Optional fields become required nullable fields on the wire only.
// The original schema remains the authority for omission versus explicit clear.
func OpenAISchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("invalid motion response schema")
	}
	strictSchema(schema)
	if rootSchemaUnion(schema) {
		// OpenAI requires an object at the root. Keep the action union intact
		// under one wire-only property; flattening it would lose the constraint
		// that action:none carries no edits. DomainOutput removes this wrapper.
		schema = map[string]any{"type": "object", "properties": map[string]any{"proposal": schema}, "required": []string{"proposal"}, "additionalProperties": false}
	}
	return json.Marshal(schema)
}

func rootSchemaUnion(schema map[string]any) bool {
	return schema["oneOf"] != nil || schema["anyOf"] != nil
}

func strictSchema(schema map[string]any) {
	strictConstant(schema)
	if alternatives, ok := schema["oneOf"]; ok {
		schema["anyOf"] = alternatives
		delete(schema, "oneOf")
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		required := schemaRequired(schema)
		keys := make([]string, 0, len(properties))
		for key, value := range properties {
			child, ok := value.(map[string]any)
			if !ok {
				continue
			}
			strictSchema(child)
			if !slices.Contains(required, key) && !schemaAllowsNull(child) {
				properties[key] = map[string]any{"anyOf": []any{child, map[string]any{"type": "null"}}}
			}
			keys = append(keys, key)
		}
		slices.Sort(keys)
		schema["required"], schema["additionalProperties"] = keys, false
	}
	if child, ok := schema["items"].(map[string]any); ok {
		strictSchema(child)
	}
	for _, name := range []string{"anyOf", "oneOf", "allOf"} {
		if children, ok := schema[name].([]any); ok {
			for _, child := range children {
				if object, ok := child.(map[string]any); ok {
					strictSchema(object)
				}
			}
		}
	}
}

// Domain action schemas use compact const-only nodes. OpenAI also requires a
// type, and empty arrays need an items schema even though no item is allowed.
func strictConstant(schema map[string]any) {
	value, ok := schema["const"]
	if !ok {
		return
	}
	switch value := value.(type) {
	case string:
		schema["type"] = "string"
	case bool:
		schema["type"] = "boolean"
	case float64:
		schema["type"] = "number"
	case nil:
		schema["type"] = "null"
	case []any:
		if len(value) == 0 {
			schema["type"], schema["items"], schema["maxItems"] = "array", map[string]any{"type": "string"}, 0
			delete(schema, "const")
		}
	case map[string]any:
		if len(value) == 0 {
			schema["type"], schema["properties"] = "object", map[string]any{}
			delete(schema, "const")
		}
	}
}

func schemaRequired(schema map[string]any) []string {
	var required []string
	if values, ok := schema["required"].([]any); ok {
		for _, value := range values {
			if key, ok := value.(string); ok {
				required = append(required, key)
			}
		}
	}
	return required
}

func schemaAllowsNull(schema map[string]any) bool {
	if schema["type"] == "null" {
		return true
	}
	if types, ok := schema["type"].([]any); ok && slices.Contains(types, any("null")) {
		return true
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if children, ok := schema[keyword].([]any); ok {
			for _, child := range children {
				if object, ok := child.(map[string]any); ok && schemaAllowsNull(object) {
					return true
				}
			}
		}
	}
	return false
}

// DomainOutput restores omission only where our original contract permitted
// omission. Required nulls remain invalid; explicit empty arrays/objects remain
// intact, including clear-layer operations. Semantic parsers still run next.
func DomainOutput(raw string, original json.RawMessage) (string, error) {
	var value any
	var schema map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil || json.Unmarshal(original, &schema) != nil {
		return "", &CloudError{Kind: "incomplete"}
	}
	if rootSchemaUnion(schema) {
		wrapper, ok := value.(map[string]any)
		if !ok || len(wrapper) != 1 || wrapper["proposal"] == nil {
			return "", &CloudError{Kind: "incomplete"}
		}
		value = wrapper["proposal"]
	}
	domainValue(value, schema)
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func domainValue(value any, schema map[string]any) {
	// Edit/action unions have different omission contracts. Restore only an
	// unambiguous branch, preserving malformed outputs for the semantic parser.
	for _, keyword := range []string{"oneOf", "anyOf"} {
		branches, _ := schema[keyword].([]any)
		var matched map[string]any
		matches := 0
		for _, branch := range branches {
			candidate, ok := branch.(map[string]any)
			if ok && domainBranchMatches(value, candidate) {
				matched = candidate
				matches++
			}
		}
		if matches == 1 {
			domainValue(value, matched)
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		required := schemaRequired(schema)
		for key, value := range object {
			child, known := properties[key].(map[string]any)
			if !known {
				continue
			}
			if value == nil && !slices.Contains(required, key) && !schemaAllowsNull(child) {
				delete(object, key)
				continue
			}
			domainValue(value, child)
		}
	}
	if array, ok := value.([]any); ok {
		child, _ := schema["items"].(map[string]any)
		for _, value := range array {
			domainValue(value, child)
		}
	}
}

func domainBranchMatches(value any, schema map[string]any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range schemaRequired(schema) {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	for key, property := range properties {
		child, ok := property.(map[string]any)
		if !ok {
			continue
		}
		actual, exists := object[key]
		if !exists {
			continue
		}
		if constant, hasConstant := child["const"]; hasConstant && !reflect.DeepEqual(actual, constant) {
			return false
		}
		if values, hasEnum := child["enum"].([]any); hasEnum && !slices.ContainsFunc(values, func(value any) bool { return reflect.DeepEqual(value, actual) }) {
			return false
		}
	}
	return true
}

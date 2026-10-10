package llm

import (
	"encoding/json"
	"testing"
)

func TestOpenAIActionUnionUsesTypedObjectEnvelope(t *testing.T) {
	for _, empty := range []string{`[]`, `{}`} {
		schema := json.RawMessage(`{"oneOf":[{"type":"object","properties":{"action":{"const":"none"},"edits":{"const":` + empty + `},"reply":{"type":"string"}},"required":["action","edits","reply"],"additionalProperties":false},{"type":"object","properties":{"action":{"const":"update"},"edits":{"type":"object","properties":{"speed":{"type":"integer"}},"additionalProperties":false},"reply":{"type":"string"}},"required":["action","edits","reply"],"additionalProperties":false}]}`)
		wire, err := OpenAISchema(schema)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]any
		if err := json.Unmarshal(wire, &root); err != nil {
			t.Fatal(err)
		}
		if root["type"] != "object" || root["anyOf"] != nil || root["additionalProperties"] != false {
			t.Fatalf("unsupported root: %s", wire)
		}
		proposal := root["properties"].(map[string]any)["proposal"].(map[string]any)
		branch := proposal["anyOf"].([]any)[0].(map[string]any)["properties"].(map[string]any)
		if branch["action"].(map[string]any)["type"] != "string" || branch["edits"].(map[string]any)["type"] == nil {
			t.Fatalf("untyped constants: %s", wire)
		}
		raw := `{"proposal":{"action":"none","edits":` + empty + `,"reply":"Holding."}}`
		domain, err := DomainOutput(raw, schema)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		_ = json.Unmarshal([]byte(domain), &decoded)
		if decoded["action"] != "none" || decoded["proposal"] != nil {
			t.Fatalf("domain envelope leaked: %s", domain)
		}
		if _, err := DomainOutput(`{"proposal":null}`, schema); err == nil {
			t.Fatal("null proposal accepted")
		}
		if _, err := DomainOutput(`{"proposal":{"action":"none"},"extra":"invalid"}`, schema); err == nil {
			t.Fatal("extra envelope fields accepted")
		}
	}
}

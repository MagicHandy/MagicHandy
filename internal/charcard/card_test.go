package charcard

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

// buildCardPNG returns a valid PNG with the payload stored in a tEXt chunk
// under keyword, inserted before IEND, the way Tavern-family tools write cards.
func buildCardPNG(t *testing.T, keyword string, payload []byte) []byte {
	t.Helper()
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	data := img.Bytes()

	encoded := base64.StdEncoding.EncodeToString(payload)
	chunk := append(append([]byte(keyword), 0), []byte(encoded)...)

	var out bytes.Buffer
	out.Write(data[:len(data)-12]) // everything before the IEND chunk
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(chunk)))
	out.Write(length[:])
	body := append([]byte("tEXt"), chunk...)
	out.Write(body)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(body))
	out.Write(crc[:])
	out.Write(data[len(data)-12:]) // IEND
	return out.Bytes()
}

func v2CardJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"spec":         "chara_card_v2",
		"spec_version": "2.0",
		"data": map[string]any{
			"name":        "Annabelle",
			"description": "A shy step-sister.",
			"personality": "Curious, stubborn.",
			"scenario":    "Late night in the kitchen.",
			"first_mes":   "*She looks up.* Oh, it's you.",
			"mes_example": "<START>\n{{char}}: Hi.",
		},
	})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	return payload
}

func TestParseJSONNormalizesV2(t *testing.T) {
	card, err := ParseJSON(v2CardJSON(t))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if card.Name != "Annabelle" {
		t.Fatalf("name = %q", card.Name)
	}
	if card.Description != "A shy step-sister." {
		t.Fatalf("description = %q", card.Description)
	}
	if card.Personality != "Curious, stubborn." {
		t.Fatalf("personality = %q", card.Personality)
	}
	if card.Scenario != "Late night in the kitchen." {
		t.Fatalf("scenario = %q", card.Scenario)
	}
	if card.Greeting != "*She looks up.* Oh, it's you." {
		t.Fatalf("greeting = %q", card.Greeting)
	}
	if !strings.Contains(card.ExampleMessages, "{{char}}: Hi.") {
		t.Fatalf("example messages = %q", card.ExampleMessages)
	}
}

func TestParseJSONNormalizesV1(t *testing.T) {
	card, err := ParseJSON([]byte(`{
		"name": "Lily",
		"description": "Cheerful.",
		"personality": "Playful",
		"scenario": "At home.",
		"first_mes": "Hallo!",
		"mes_example": ""
	}`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if card.Name != "Lily" || card.Greeting != "Hallo!" || card.Scenario != "At home." {
		t.Fatalf("card = %+v", card)
	}
}

func TestParseJSONPrefersV3DataOverV1Mirror(t *testing.T) {
	card, err := ParseJSON([]byte(`{
		"spec": "chara_card_v3",
		"name": "Old",
		"description": "old",
		"data": {"name": "New", "description": "new"}
	}`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if card.Name != "New" || card.Description != "new" {
		t.Fatalf("card = %+v", card)
	}
}

func TestParseJSONRejectsMissingName(t *testing.T) {
	if _, err := ParseJSON([]byte(`{"description": "no name"}`)); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseJSONRejectsGarbage(t *testing.T) {
	if _, err := ParseJSON([]byte("not json")); err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestParsePNGReadsCharaChunk(t *testing.T) {
	data := buildCardPNG(t, "chara", v2CardJSON(t))
	card, err := ParsePNG(data)
	if err != nil {
		t.Fatalf("ParsePNG: %v", err)
	}
	if card.Name != "Annabelle" {
		t.Fatalf("name = %q", card.Name)
	}
}

func TestParsePNGPrefersCCV3OverChara(t *testing.T) {
	v3, err := json.Marshal(map[string]any{
		"spec": "chara_card_v3",
		"data": map[string]any{"name": "V3 Name"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data := buildCardPNG(t, "chara", v2CardJSON(t))
	// Rebuild with both chunks: chara first, ccv3 after.
	data = insertBeforeIEND(t, data, textChunk(t, "ccv3", v3))
	card, err := ParsePNG(data)
	if err != nil {
		t.Fatalf("ParsePNG: %v", err)
	}
	if card.Name != "V3 Name" {
		t.Fatalf("name = %q, want ccv3 payload to win", card.Name)
	}
}

func TestParsePNGWithoutCardChunkFails(t *testing.T) {
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := ParsePNG(img.Bytes()); err == nil {
		t.Fatal("expected error for PNG without card data")
	}
}

func TestParseSniffsPNGAndJSON(t *testing.T) {
	fromPNG, err := Parse(buildCardPNG(t, "chara", v2CardJSON(t)))
	if err != nil {
		t.Fatalf("Parse png: %v", err)
	}
	fromJSON, err := Parse(v2CardJSON(t))
	if err != nil {
		t.Fatalf("Parse json: %v", err)
	}
	if fromPNG.Name != fromJSON.Name {
		t.Fatalf("png %q vs json %q", fromPNG.Name, fromJSON.Name)
	}
}

func TestReplaceMacros(t *testing.T) {
	cases := []struct{ in, want string }{
		{"{{char}} smiles at {{user}}.", "Lily smiles at you."},
		{"{{User}}'s coffee is cold.", "your coffee is cold."},
		{"{{CHAR}} waves.", "Lily waves."},
		{"no macros", "no macros"},
	}
	for _, c := range cases {
		if got := ReplaceMacros(c.in, "Lily"); got != c.want {
			t.Fatalf("ReplaceMacros(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// textChunk builds one tEXt chunk (length + type + data + crc).
func textChunk(t *testing.T, keyword string, payload []byte) []byte {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString(payload)
	body := append(append(append([]byte("tEXt"), []byte(keyword)...), 0), []byte(encoded)...)
	var out bytes.Buffer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(body)-4))
	out.Write(length[:])
	out.Write(body)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(body))
	out.Write(crc[:])
	return out.Bytes()
}

func insertBeforeIEND(t *testing.T, data []byte, chunk []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	out.Write(data[:len(data)-12])
	out.Write(chunk)
	out.Write(data[len(data)-12:])
	return out.Bytes()
}

// Package charcard parses Tavern-family character cards: the portable
// character format used across roleplay card sites, carried either as plain
// JSON or embedded in a PNG tEXt chunk. Parsing is stdlib-only and yields a
// normalized Card; how card text is bounded and composed into prompts is the
// importing package's concern, not this one's.
package charcard

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Errors distinguish "not a card" flavors so the API can keep messages exact.
var (
	// ErrNoCardData reports a PNG without an embedded character payload.
	ErrNoCardData = errors.New("no character card data found in PNG")
	// ErrInvalidCard reports a payload that is not a usable character card.
	ErrInvalidCard = errors.New("invalid character card")
)

// Card is the normalized character description shared by card spec versions
// V1 through V3. Field text is raw card content: macros are not yet replaced
// and no length bounds are applied.
type Card struct {
	Name            string
	Description     string
	Personality     string
	Scenario        string
	Greeting        string
	ExampleMessages string
	CreatorNotes    string
}

// cardFields is the on-disk field set shared by V1 (top level) and V2/V3
// (nested under data). Unknown fields are deliberately ignored.
type cardFields struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Personality     string `json:"personality"`
	Scenario        string `json:"scenario"`
	FirstMes        string `json:"first_mes"`
	MesExample      string `json:"mes_example"`
	CreatorNotes    string `json:"creator_notes"`
	CreatorComment  string `json:"creatorcomment"`
	Data            *cardFields
}

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Parse sniffs the payload and dispatches to ParsePNG or ParseJSON.
func Parse(data []byte) (Card, error) {
	if bytes.HasPrefix(data, pngMagic) {
		return ParsePNG(data)
	}
	return ParseJSON(data)
}

// ParseJSON normalizes a V1, V2, or V3 card document. V2/V3 nest the real
// fields under data with a V1 mirror at the top level; the nested copy wins.
func ParseJSON(data []byte) (Card, error) {
	var fields cardFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return Card{}, fmt.Errorf("%w: %v", ErrInvalidCard, err)
	}
	if fields.Data != nil {
		fields = *fields.Data
	}
	card := Card{
		Name:            strings.TrimSpace(fields.Name),
		Description:     strings.TrimSpace(fields.Description),
		Personality:     strings.TrimSpace(fields.Personality),
		Scenario:        strings.TrimSpace(fields.Scenario),
		Greeting:        strings.TrimSpace(fields.FirstMes),
		ExampleMessages: strings.TrimSpace(fields.MesExample),
		CreatorNotes:    strings.TrimSpace(fields.CreatorNotes),
	}
	if card.CreatorNotes == "" {
		card.CreatorNotes = strings.TrimSpace(fields.CreatorComment)
	}
	if card.Name == "" {
		return Card{}, fmt.Errorf("%w: card has no character name", ErrInvalidCard)
	}
	return card, nil
}

// ParsePNG extracts the card payload from a PNG. V3 writers use a tEXt chunk
// keyed ccv3; V2-era writers (and V3 writers keeping compatibility) use chara.
// Both carry base64 JSON. ccv3 wins when both are present.
func ParsePNG(data []byte) (Card, error) {
	if !bytes.HasPrefix(data, pngMagic) {
		return Card{}, fmt.Errorf("%w: not a PNG file", ErrInvalidCard)
	}
	var charaPayload, ccv3Payload []byte
	pos := len(pngMagic)
	for pos+12 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		chunkType := string(data[pos+4 : pos+8])
		bodyEnd := pos + 8 + length
		if length < 0 || bodyEnd+4 > len(data) {
			break
		}
		if chunkType == "tEXt" {
			body := data[pos+8 : bodyEnd]
			keyword, value, found := bytes.Cut(body, []byte{0})
			if found {
				switch string(keyword) {
				case "chara":
					charaPayload = value
				case "ccv3":
					ccv3Payload = value
				}
			}
		}
		pos = bodyEnd + 4
	}
	payload := ccv3Payload
	if payload == nil {
		payload = charaPayload
	}
	if payload == nil {
		return Card{}, ErrNoCardData
	}
	decoded, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return Card{}, fmt.Errorf("%w: card payload is not valid base64", ErrInvalidCard)
	}
	return ParseJSON(decoded)
}

// Macro replacement: cards address the character as {{char}} and the reader as
// {{user}}. MagicHandy has no user display name, so possessive {{user}}'s
// becomes "your" and bare {{user}} becomes "you".
var (
	userPossessiveMacro = regexp.MustCompile(`(?i)\{\{user\}\}['']s`)
	userMacro           = regexp.MustCompile(`(?i)\{\{user\}\}`)
	charMacro           = regexp.MustCompile(`(?i)\{\{char\}\}`)
)

// ReplaceMacros resolves the two universal card macros against name.
func ReplaceMacros(text string, name string) string {
	text = charMacro.ReplaceAllLiteralString(text, name)
	text = userPossessiveMacro.ReplaceAllLiteralString(text, "your")
	return userMacro.ReplaceAllLiteralString(text, "you")
}

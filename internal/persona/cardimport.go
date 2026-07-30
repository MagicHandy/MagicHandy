package persona

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // card art arrives as PNG
	"strings"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/charcard"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

// ImportCard converts a parsed character card into a persona with lore and an
// optional portrait. Card text is bounded to the same budgets hand-written
// personas live under — those bounds are the app's motion-adherence budget,
// not an import limitation — and every truncation is reported back so the
// user can see what did not fit. Unusable art degrades to a warning: a
// persona with a monogram beats a failed import.
func (s *Store) ImportCard(
	ctx context.Context,
	card charcard.Card,
	artPNG []byte,
) (Persona, []string, error) {
	var warnings []string
	clip := func(value string, limit int, field string) string {
		value = strings.TrimSpace(value)
		if utf8.RuneCountInString(value) <= limit {
			return value
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s was longer than %d characters and was shortened", field, limit))
		return strings.TrimSpace(string([]rune(value)[:limit]))
	}

	name := clip(collapseSpaces(charcard.ReplaceMacros(card.Name, card.Name)), MaxNameChars, "name")
	description := collapseSpaces(charcard.ReplaceMacros(card.Description, name))
	greeting := clip(charcard.ReplaceMacros(card.Greeting, name), MaxGreetingChars, "greeting")

	descriptionOverflow := ""
	if utf8.RuneCountInString(description) > MaxDescriptionChars {
		runes := []rune(description)
		descriptionOverflow = strings.TrimSpace(string(runes[MaxDescriptionChars:]))
		description = strings.TrimSpace(string(runes[:MaxDescriptionChars]))
		warnings = append(warnings, fmt.Sprintf(
			"description was longer than %d characters; the rest was moved into lore", MaxDescriptionChars))
	}

	lore, loreWarnings := loreFromCard(name, card, descriptionOverflow)
	warnings = append(warnings, loreWarnings...)

	now := timestamp()
	item := Persona{
		ID:               idPrefix + randomHex(idHexDigits/2),
		Name:             name,
		Description:      description,
		ChatVoice:        config.LLMChatVoiceWarm,
		ReactionStyle:    config.LLMReactionStyleNeutral,
		DefaultFocusArea: chat.AreaZoneFull,
		LoreMode:         LoreModeOff,
		Greeting:         greeting,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if len(lore) > 0 {
		item.LoreMode = LoreModeFull
	}
	if err := validate(item); err != nil {
		return Persona{}, nil, err
	}

	s.mu.Lock()
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM personas").Scan(&count); err != nil {
			return err
		}
		if count >= maxPersonas {
			return fmt.Errorf("%w (%d)", ErrLimit, maxPersonas)
		}
		if err := insert(ctx, tx, item); err != nil {
			return err
		}
		for _, text := range lore {
			keywords, _ := json.Marshal([]string{})
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO persona_lore(
					id, persona_id, text, keywords_json, enabled, created_at, updated_at
				) VALUES(?, ?, ?, ?, 1, ?, ?)
			`, loreIDPrefix+randomHex(loreIDHexDigits/2), item.ID, text, string(keywords), now, now); err != nil {
				return err
			}
		}
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return Persona{}, nil, err
	}
	item.LoreCount = len(lore)

	if len(artPNG) > 0 {
		portrait, convertErr := portraitJPEGFromArt(artPNG)
		if convertErr == nil {
			if saved, saveErr := s.SavePortrait(ctx, item.ID, portrait); saveErr == nil {
				item = saved
			} else {
				convertErr = saveErr
			}
		}
		if convertErr != nil {
			warnings = append(warnings, "the card art could not be used as a portrait")
		}
	}
	return item, warnings, nil
}

// loreFromCard fills the lore budget in priority order: personality, scenario,
// description overflow, then example messages. Later sources are the first to
// be dropped when the budget runs out.
func loreFromCard(name string, card charcard.Card, descriptionOverflow string) ([]string, []string) {
	sources := []struct {
		label string
		text  string
	}{
		{"personality", collapseSpaces(charcard.ReplaceMacros(card.Personality, name))},
		{"scenario", collapseSpaces(charcard.ReplaceMacros(card.Scenario, name))},
		{"description", descriptionOverflow},
		{"example messages", collapseSpaces(charcard.ReplaceMacros(card.ExampleMessages, name))},
	}

	var entries []string
	var warnings []string
	remaining := MaxLoreTotalChars
	for _, source := range sources {
		text := strings.TrimSpace(source.text)
		if text == "" {
			continue
		}
		truncated := false
		for text != "" && len(entries) < MaxLoreEntries && remaining > 0 {
			limit := MaxLoreTextChars
			if remaining < limit {
				limit = remaining
			}
			chunk, rest := splitAtRuneLimit(text, limit)
			if chunk == "" {
				break
			}
			entries = append(entries, chunk)
			remaining -= utf8.RuneCountInString(chunk)
			text = rest
		}
		if text != "" {
			truncated = true
		}
		if truncated {
			warnings = append(warnings, fmt.Sprintf(
				"the card's %s did not fit the lore budget and was shortened", source.label))
		}
	}
	return entries, warnings
}

// splitAtRuneLimit cuts text at the last space before limit runes, falling
// back to a hard cut when the text has no usable break.
func splitAtRuneLimit(text string, limit int) (string, string) {
	runes := []rune(text)
	if len(runes) <= limit {
		return strings.TrimSpace(text), ""
	}
	cut := limit
	for i := limit; i > limit/2; i-- {
		if runes[i-1] == ' ' {
			cut = i - 1
			break
		}
	}
	head := strings.TrimSpace(string(runes[:cut]))
	tail := strings.TrimSpace(string(runes[cut:]))
	return head, tail
}

// portraitJPEGFromArt turns card art into what the portrait store accepts: a
// JPEG within MaxPortraitEdge and MaxPortraitBytes. Scaling is a plain box
// average — pure Go, no new dependency, and portrait-sized output.
func portraitJPEGFromArt(art []byte) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(art))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPortraitInvalid, err)
	}
	scaled := scaleWithin(source, MaxPortraitEdge)
	for _, quality := range []int{85, 70, 50} {
		var buffer bytes.Buffer
		if err := jpeg.Encode(&buffer, scaled, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPortraitInvalid, err)
		}
		if buffer.Len() <= MaxPortraitBytes {
			return buffer.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("%w: converted portrait stays too large", ErrPortraitInvalid)
}

// scaleWithin box-averages source down so both edges fit maxEdge. A source
// already within bounds is returned untouched.
func scaleWithin(source image.Image, maxEdge int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxEdge && height <= maxEdge {
		return source
	}
	var targetWidth, targetHeight int
	if width >= height {
		targetWidth = maxEdge
		targetHeight = height * maxEdge / width
	} else {
		targetHeight = maxEdge
		targetWidth = width * maxEdge / height
	}
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}

	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY0 := bounds.Min.Y + y*height/targetHeight
		sourceY1 := bounds.Min.Y + (y+1)*height/targetHeight
		if sourceY1 <= sourceY0 {
			sourceY1 = sourceY0 + 1
		}
		for x := 0; x < targetWidth; x++ {
			sourceX0 := bounds.Min.X + x*width/targetWidth
			sourceX1 := bounds.Min.X + (x+1)*width/targetWidth
			if sourceX1 <= sourceX0 {
				sourceX1 = sourceX0 + 1
			}
			var r, g, b, count uint64
			for sy := sourceY0; sy < sourceY1; sy++ {
				for sx := sourceX0; sx < sourceX1; sx++ {
					pr, pg, pb, _ := source.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					count++
				}
			}
			target.SetRGBA(x, y, color.RGBA{
				R: channelByte(r / count),
				G: channelByte(g / count),
				B: channelByte(b / count),
				A: 0xFF,
			})
		}
	}
	return target
}

// channelByte folds one averaged 16-bit color channel into its 8-bit form.
func channelByte(value uint64) uint8 {
	value >>= 8
	if value > 0xFF {
		value = 0xFF
	}
	return uint8(value)
}

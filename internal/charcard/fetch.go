package charcard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	// MaxFetchBytes bounds one downloaded resource. Cards with embedded art
	// run a few megabytes; anything past this is not a card.
	MaxFetchBytes = 16 << 20
	// maxLinkCandidates bounds how many linked card files one page fetch may
	// follow. One page, one extra hop, a handful of tries.
	maxLinkCandidates = 3
	// maxEmbeddedScan bounds how far back from a card marker the embedded-JSON
	// scan walks looking for the object start.
	maxEmbeddedScan = 64
)

// ErrFetchFailed reports a network or HTTP-level failure, distinct from a
// reachable page that simply carries no card data.
var ErrFetchFailed = errors.New("character card download failed")

// FetchResult is a parsed card plus the card art it arrived in, when the
// source was a card PNG.
type FetchResult struct {
	Card        Card
	PortraitPNG []byte
	SourceURL   string
}

// Fetch downloads rawURL and extracts a character card. Direct PNG and JSON
// card files parse as-is. An HTML page is scanned for embedded card JSON and,
// failing that, for a same-host link to a card file (one hop). A reachable
// page with no discoverable card data returns ErrNoCardData: sites that only
// show characters to logged-in members look exactly like that.
func Fetch(ctx context.Context, client *http.Client, rawURL string) (FetchResult, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return FetchResult{}, fmt.Errorf("%w: only http and https URLs are supported", ErrInvalidCard)
	}

	body, err := fetchBytes(ctx, client, parsed.String())
	if err != nil {
		return FetchResult{}, err
	}

	if result, ok := parseDirect(body, parsed.String()); ok {
		return result, nil
	}

	page := string(body)
	if card, ok := extractEmbeddedCard(page); ok {
		return FetchResult{Card: card, SourceURL: parsed.String()}, nil
	}
	for _, link := range cardFileLinks(parsed, page) {
		linked, err := fetchBytes(ctx, client, link)
		if err != nil {
			continue
		}
		if result, ok := parseDirect(linked, link); ok {
			return result, nil
		}
	}
	return FetchResult{}, fmt.Errorf(
		"%w at this URL; the site may require a login — download the card file and import it instead",
		ErrNoCardData,
	)
}

func fetchBytes(ctx context.Context, client *http.Client, target string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	request.Header.Set("User-Agent", "MagicHandy/1.0 (character card import)")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: server responded %s", ErrFetchFailed, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxFetchBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	if len(body) > MaxFetchBytes {
		return nil, fmt.Errorf("%w: response is larger than %d bytes", ErrFetchFailed, MaxFetchBytes)
	}
	return body, nil
}

// parseDirect handles the two self-contained card payloads: a card PNG (which
// doubles as the portrait) and a card JSON document.
func parseDirect(body []byte, source string) (FetchResult, bool) {
	if bytes.HasPrefix(body, pngMagic) {
		card, err := ParsePNG(body)
		if err != nil {
			return FetchResult{}, false
		}
		return FetchResult{Card: card, PortraitPNG: body, SourceURL: source}, true
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("{")) {
		card, err := ParseJSON(trimmed)
		if err != nil {
			return FetchResult{}, false
		}
		return FetchResult{Card: card, SourceURL: source}, true
	}
	return FetchResult{}, false
}

var embeddedCardMarkers = []string{`chara_card_v`, `"first_mes"`}

// extractEmbeddedCard finds card JSON inside an HTML page. From each marker
// occurrence it walks backward over candidate object starts and tries to parse
// the balanced JSON object beginning there.
func extractEmbeddedCard(page string) (Card, bool) {
	for _, marker := range embeddedCardMarkers {
		offset := 0
		for {
			at := strings.Index(page[offset:], marker)
			if at < 0 {
				break
			}
			at += offset
			if card, ok := cardAroundMarker(page, at); ok {
				return card, true
			}
			offset = at + len(marker)
		}
	}
	return Card{}, false
}

func cardAroundMarker(page string, marker int) (Card, bool) {
	start := marker
	for tries := 0; tries < maxEmbeddedScan; tries++ {
		start = strings.LastIndexByte(page[:start], '{')
		if start < 0 {
			return Card{}, false
		}
		object, ok := balancedObject(page, start)
		if !ok {
			continue
		}
		card, err := ParseJSON([]byte(object))
		if err == nil {
			return card, true
		}
	}
	return Card{}, false
}

// balancedObject returns the JSON object starting at start, honoring strings
// and escapes so braces inside card text do not end the object early.
func balancedObject(page string, start int) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(page); i++ {
		c := page[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return page[start : i+1], true
			}
		}
	}
	return "", false
}

var hrefPattern = regexp.MustCompile(`(?i)href\s*=\s*"([^"]+\.(?:png|json)(?:\?[^"]*)?)"`)

// cardFileLinks resolves page links that plausibly point at a card file,
// keeping only same-host targets so one import cannot fan out across sites.
func cardFileLinks(page *url.URL, body string) []string {
	var links []string
	for _, match := range hrefPattern.FindAllStringSubmatch(body, maxLinkCandidates*4) {
		target, err := page.Parse(match[1])
		if err != nil || target.Host != page.Host {
			continue
		}
		links = append(links, target.String())
		if len(links) >= maxLinkCandidates {
			break
		}
	}
	return links
}

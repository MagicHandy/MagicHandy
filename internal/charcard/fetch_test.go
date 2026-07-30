package charcard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fetchFrom(t *testing.T, server *httptest.Server, path string) (FetchResult, error) {
	t.Helper()
	return Fetch(context.Background(), server.Client(), server.URL+path)
}

func TestFetchDirectPNGCard(t *testing.T) {
	card := buildCardPNG(t, "chara", v2CardJSON(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(card)
	}))
	defer server.Close()

	result, err := fetchFrom(t, server, "/card.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.Card.Name != "Annabelle" {
		t.Fatalf("name = %q", result.Card.Name)
	}
	if len(result.PortraitPNG) == 0 {
		t.Fatal("expected the card PNG itself as portrait")
	}
}

func TestFetchDirectJSONCard(t *testing.T) {
	payload := v2CardJSON(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	result, err := fetchFrom(t, server, "/card.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.Card.Name != "Annabelle" {
		t.Fatalf("name = %q", result.Card.Name)
	}
	if len(result.PortraitPNG) != 0 {
		t.Fatal("JSON card has no portrait")
	}
}

func TestFetchHTMLWithEmbeddedCardJSON(t *testing.T) {
	embedded, err := json.Marshal(map[string]any{
		"spec": "chara_card_v2",
		"data": map[string]any{
			"name":        "Embedded",
			"description": "From a script tag.",
			"first_mes":   "Hello.",
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	page := `<html><head><title>x</title></head><body>
		<script type="application/json">{"page":"state","nested":` + string(embedded) + `}</script>
	</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	result, err := fetchFrom(t, server, "/character/1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.Card.Name != "Embedded" || result.Card.Greeting != "Hello." {
		t.Fatalf("card = %+v", result.Card)
	}
}

func TestFetchHTMLFollowsCardFileLink(t *testing.T) {
	card := buildCardPNG(t, "chara", v2CardJSON(t))
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="/downloads/card.png">Download card</a></body></html>`))
	})
	mux.HandleFunc("/downloads/card.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(card)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := fetchFrom(t, server, "/page")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.Card.Name != "Annabelle" {
		t.Fatalf("name = %q", result.Card.Name)
	}
}

func TestFetchHTMLWithoutCardDataFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><p>Log in to view this character.</p></body></html>`))
	}))
	defer server.Close()

	_, err := fetchFrom(t, server, "/character/1")
	if err == nil {
		t.Fatal("expected error for page without card data")
	}
	if !strings.Contains(err.Error(), "no character card") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRejectsNonHTTPSchemes(t *testing.T) {
	if _, err := Fetch(context.Background(), http.DefaultClient, "file:///etc/passwd"); err == nil {
		t.Fatal("expected scheme rejection")
	}
}

func TestFetchRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		chunk := make([]byte, 1<<20)
		for range 20 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	if _, err := fetchFrom(t, server, "/huge.png"); err == nil {
		t.Fatal("expected size rejection")
	}
}

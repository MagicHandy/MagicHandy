package httpapi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHashedAssetVariantsAndConditionalRequests(t *testing.T) {
	content := []byte(strings.Repeat("const sample = 42;\n", 100))
	server := &Server{static: fstest.MapFS{"assets/app-abcdefgh.js": {Data: content}, "index.html": {Data: []byte("<html></html>")}}}
	get := func(path, encoding, etag, byteRange string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Accept-Encoding", encoding)
		r.Header.Set("If-None-Match", etag)
		r.Header.Set("Range", byteRange)
		w := httptest.NewRecorder()
		server.handleStatic(w, r)
		return w
	}
	path := "/assets/app-abcdefgh.js"
	plain := get(path, "gzip;q=0, *;q=1", "", "")
	compressed := get(path, "br, gzip", "", "")
	if plain.Header().Get("Content-Encoding") != "" || !bytes.Equal(plain.Body.Bytes(), content) || compressed.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(compressed.Header().Get("Cache-Control"), "immutable") || !strings.Contains(compressed.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatal("asset negotiation or immutable cache policy is wrong")
	}
	z, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(z)
	if err != nil || !bytes.Equal(content, got) {
		t.Fatal("compressed bytes differed")
	}
	_ = z.Close()
	a, b := plain.Header().Get("ETag"), compressed.Header().Get("ETag")
	if a == "" || a == b || get(path, "gzip", b, "").Code != 304 || get(path, "gzip", a, "").Code != 200 || get(path, "", a, "").Code != 304 {
		t.Fatal("conditional requests mixed representation validators")
	}
	if partial := get(path, "", "", "bytes=0-4"); partial.Code != 206 || partial.Body.String() != "const" {
		t.Fatal("identity range lost")
	}
	if page := get("/settings", "", "", ""); page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("SPA shell was cached as an immutable asset")
	}
}

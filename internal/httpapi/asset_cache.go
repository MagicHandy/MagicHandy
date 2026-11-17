package httpapi

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Vite names content-addressed script and style files with a terminal hash.
var hashedBrowserAsset = regexp.MustCompile(`^assets/[A-Za-z0-9_.-]+-[A-Za-z0-9_-]{8}\.(js|css)$`)

type assetRepresentation struct {
	bytes          []byte
	etag, encoding string
}

type compressedAssets struct {
	mu   sync.Mutex
	gzip map[string]assetRepresentation
}

func (c *compressedAssets) representation(assets fs.FS, name string, compress bool) (assetRepresentation, error) {
	etag := strconv.Quote(name)
	if !compress {
		data, err := fs.ReadFile(assets, name)
		return assetRepresentation{bytes: data, etag: etag}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.gzip[name]; ok {
		return existing, nil
	}
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		return assetRepresentation{}, err
	}
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err = writer.Write(data); err != nil {
		return assetRepresentation{}, err
	}
	if err = writer.Close(); err != nil {
		return assetRepresentation{}, err
	}
	variant := assetRepresentation{bytes: buf.Bytes(), etag: strconv.Quote(name + "-gzip"), encoding: "gzip"}
	if c.gzip == nil {
		c.gzip = make(map[string]assetRepresentation)
	}
	c.gzip[name] = variant
	return variant, nil
}

func acceptsGzip(headers []string) bool {
	wildcard := false
	for _, entry := range strings.Split(strings.Join(headers, ","), ",") {
		name, params, _ := strings.Cut(entry, ";")
		quality := 1.0
		for _, parameter := range strings.Split(params, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(key, "q") {
				quality, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
			}
		}
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			return quality > 0 && quality <= 1
		}
		if strings.TrimSpace(name) == "*" {
			wildcard = quality > 0 && quality <= 1
		}
	}
	return wildcard
}

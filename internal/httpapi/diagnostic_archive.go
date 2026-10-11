package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
)

const diagnosticReportMaxBytes = 64 << 20

var errDiagnosticReportTooLarge = errors.New("diagnostic report exceeds 64 MiB; exclude full chat history or optional content and try again")

// diagnosticArchive writes selected, redacted JSON directly into a ZIP. Chat
// history is paged, so a complete export need not live in the Go/browser heap.
// The uncompressed bound applies across all entries, including the manifest.
type diagnosticArchive struct {
	zip       *zip.Writer
	ctx       context.Context
	redact    func(string) string
	remaining int
	files     []string
	notes     []string
}

type diagnosticEntry struct {
	archive *diagnosticArchive
	writer  io.Writer
}

func (entry diagnosticEntry) Write(data []byte) (int, error) {
	if err := entry.archive.ctx.Err(); err != nil {
		return 0, err
	}
	if len(data) > entry.archive.remaining {
		return 0, errDiagnosticReportTooLarge
	}
	n, err := entry.writer.Write(data)
	entry.archive.remaining -= n
	return n, err
}

func (a *diagnosticArchive) entry(name string) (io.Writer, error) {
	writer, err := a.zip.Create(name)
	if err != nil {
		return nil, err
	}
	a.files = append(a.files, name)
	return diagnosticEntry{archive: a, writer: writer}, nil
}

func (a *diagnosticArchive) addJSON(name string, value any) error {
	writer, err := a.entry(name)
	if err != nil {
		return err
	}
	return a.writeJSON(writer, value)
}

// Redact decoded string values, not JSON source: escaped credentials must not
// survive, and replacing a URL or sensitive line must never corrupt the file.
func (a *diagnosticArchive) writeJSON(writer io.Writer, value any) error {
	if err := a.ctx.Err(); err != nil {
		return err
	}
	document, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(document) > a.remaining {
		return errDiagnosticReportTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	return json.NewEncoder(writer).Encode(redactDiagnosticValue(decoded, a.redact))
}

func redactDiagnosticValue(value any, redact func(string) string) any {
	switch item := value.(type) {
	case string:
		return redact(item)
	case []any:
		for i := range item {
			item[i] = redactDiagnosticValue(item[i], redact)
		}
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[redact(key)] = redactDiagnosticValue(child, redact)
		}
		return result
	}
	return value
}

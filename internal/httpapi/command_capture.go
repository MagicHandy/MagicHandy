package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
)

// The request body is hashed while the existing bounded decoder reads it.
// No command payload, upload or credentials are copied into the receipt store.
type commandBody struct {
	io.ReadCloser
	hash     hash.Hash
	count    int64
	complete bool
}

func newCommandBody(r *http.Request) *commandBody {
	body := &commandBody{ReadCloser: r.Body, hash: sha256.New(), complete: r.ContentLength == 0}
	_, _ = io.WriteString(body.hash, r.Method+"\x00"+r.URL.RequestURI()+"\x00")
	return body
}

func (b *commandBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	_, _ = b.hash.Write(data[:n])
	b.count += int64(n)
	if err == io.EOF {
		b.complete = true
	}
	return n, err
}

func (b *commandBody) fingerprint() string { return hex.EncodeToString(b.hash.Sum(nil)) }

func (b *commandBody) matches(receipt *commandReceipt) bool {
	if receipt.fingerprint == "" {
		return false
	}
	if !b.complete {
		// Consume at most the original size plus one byte. A reused ID with a
		// larger or different body never executes a second command.
		_, _ = io.CopyN(io.Discard, b, max(0, receipt.bodyBytes-b.count)+1)
	}
	return b.complete && b.count == receipt.bodyBytes && b.fingerprint() == receipt.fingerprint
}

type commandResponse struct {
	http.ResponseWriter
	status                          int
	data                            []byte
	streamed, overflow, interrupted bool
}

func (w *commandResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *commandResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *commandResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.overflow && !w.streamed {
		if len(w.data)+len(data) > maxCommandReplyBytes {
			w.overflow, w.data = true, nil
		} else {
			w.data = append(w.data, data...)
		}
	}
	return w.ResponseWriter.Write(data)
}
func (w *commandResponse) Flush() { _ = w.FlushError() }
func (w *commandResponse) FlushError() error {
	w.streamed, w.data = true, nil
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

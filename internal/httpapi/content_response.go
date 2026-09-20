package httpapi

import (
	"context"
	"io"
	"net/http"
	"time"
)

const contentWriteChunkBytes = 64 << 10

// contentResponseWriter bounds each socket write, not the total transfer time.
// A progressing video/download can last indefinitely; a stalled receiver
// cannot retain its file, session and admission slot indefinitely.
type contentResponseWriter struct {
	http.ResponseWriter
	ctx context.Context
	err error
}

func (w *contentResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *contentResponseWriter) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		if w.err != nil {
			return written, w.err
		}
		chunk := data[:min(len(data), contentWriteChunkBytes)]
		n, err := writeContextChunk(w.ctx, w.ResponseWriter, chunk)
		written += n
		if err != nil {
			w.err = err
			return written, err
		}
		data = data[n:]
	}
	return written, nil
}

func (w *contentResponseWriter) finish() {
	if w.err == nil {
		w.err = writeContextBytes(w.ctx, w.ResponseWriter, nil)
	}
	if w.err != nil {
		panic(http.ErrAbortHandler)
	}
}

func serveBoundedContent(w http.ResponseWriter, r *http.Request, name string, modified time.Time, content io.ReadSeeker) {
	bounded := &contentResponseWriter{ResponseWriter: w, ctx: r.Context()}
	http.ServeContent(bounded, r, name, modified, content)
	bounded.finish()
}

func writeBoundedAttachment(w http.ResponseWriter, r *http.Request, data []byte) {
	bounded := &contentResponseWriter{ResponseWriter: w, ctx: r.Context()}
	_, _ = bounded.Write(data)
	bounded.finish()
}

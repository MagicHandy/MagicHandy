package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type discardContentResponse struct{ headers http.Header }

func (w discardContentResponse) Header() http.Header         { return w.headers }
func (w discardContentResponse) WriteHeader(int)             {}
func (w discardContentResponse) Write(p []byte) (int, error) { return len(p), nil }
func (w discardContentResponse) Flush()                      {}
func (w discardContentResponse) SetWriteDeadline(time.Time) error {
	return nil
}

// This measures only handler CPU/allocations with an in-memory sink. It is not
// a network throughput, socket-buffer, TLS or physical-device measurement.
func BenchmarkContentWriteOverhead(b *testing.B) {
	data := make([]byte, 1<<20)
	for _, bounded := range []bool{false, true} {
		name := "ServeContent"
		if bounded {
			name = "BoundedContent"
		}
		b.Run(name, func(b *testing.B) {
			r := httptest.NewRequest(http.MethodGet, "/data.bin", nil)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				w := discardContentResponse{make(http.Header)}
				content := bytes.NewReader(data)
				if bounded {
					serveBoundedContent(w, r, "data.bin", time.Time{}, content)
				} else {
					http.ServeContent(w, r, "data.bin", time.Time{}, content)
				}
			}
		})
	}
}

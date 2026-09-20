package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type virtualContent struct{ position, size int64 }

func (v *virtualContent) Read(p []byte) (int, error) {
	if v.position >= v.size {
		return 0, io.EOF
	}
	n := min(len(p), int(v.size-v.position))
	clear(p[:n])
	v.position += int64(n)
	return n, nil
}
func (v *virtualContent) Seek(offset int64, whence int) (int64, error) {
	base := int64(0)
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = v.position
	case io.SeekEnd:
		base = v.size
	default:
		return 0, errors.New("invalid seek")
	}
	if base+offset < 0 {
		return 0, errors.New("negative seek")
	}
	v.position = base + offset
	return v.position, nil
}

func TestStalledContentReleasesRequestCapacityOnHTTP1AndHTTP2(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		name := "HTTP1"
		if http2 {
			name = "HTTP2"
		}
		t.Run(name, func(t *testing.T) {
			s := &Server{}
			done := make(chan struct{})
			admitted := s.admitHTTPRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveBoundedContent(w, r, "large.bin", time.Time{}, &virtualContent{size: 64 << 20})
			}))
			host := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); admitted.ServeHTTP(w, r) }))
			host.EnableHTTP2 = http2
			host.StartTLS()
			defer host.Close()
			client := host.Client()
			// A large OS receive window can keep accepting data after this
			// client stops reading. Bound that buffering so the assertion tests
			// the blocked-write deadline, not time spent filling autotuned TCP
			// buffers under the race detector and concurrent package load.
			client.Transport.(*http.Transport).DialContext = dialSmallContentWindow
			response, err := client.Get(host.URL + "/api/large-content")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if http2 && response.ProtoMajor != 2 {
				t.Fatal("fixture did not negotiate HTTP/2")
			}
			// Deliberately do not read the large response. Flow control/socket
			// buffers eventually fill and the per-write deadline must release it.
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				t.Fatal("stalled reader retained its handler")
			}
			if s.requestAdmission.snapshot()[ordinaryLane].Active != 0 {
				t.Fatal("stalled content retained request capacity")
			}
		})
	}
}

func dialSmallContentWindow(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("content fixture requires a TCP connection")
	}
	if err := tcp.SetReadBuffer(4 << 10); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func TestContentDeadlineDoesNotLimitAnIdleProducerGap(t *testing.T) {
	host := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bounded := &contentResponseWriter{ResponseWriter: w, ctx: r.Context()}
		_, _ = bounded.Write([]byte("before\n"))
		timer := time.NewTimer(5500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		_, _ = bounded.Write([]byte("after\n"))
		bounded.finish()
	}))
	host.EnableHTTP2 = true
	host.StartTLS()
	defer host.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := host.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "before\nafter\n" {
		t.Fatalf("idle producer lost a healthy stream: %q %v", data, err)
	}
}

type cancelOnContentDeadline struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w cancelOnContentDeadline) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		w.cancel()
	}
	return nil
}

func TestContentCancellationCannotBeOverwrittenByANewWriteDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	underlying := httptest.NewRecorder()
	bounded := &contentResponseWriter{ResponseWriter: cancelOnContentDeadline{underlying, cancel}, ctx: ctx}
	n, err := bounded.Write([]byte("private content"))
	if n != 0 || !errors.Is(err, context.Canceled) || underlying.Body.Len() != 0 {
		t.Fatal("content escaped after cancellation")
	}
}

func TestBoundedContentPreservesRangeAndHead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRequest(method, "/content", nil)
		r.Header.Set("Range", "bytes=2-5")
		w := httptest.NewRecorder()
		serveBoundedContent(w, r, "data.bin", time.Time{}, bytes.NewReader([]byte("0123456789")))
		if w.Code != http.StatusPartialContent || w.Header().Get("Content-Range") != "bytes 2-5/10" {
			t.Fatal("Range metadata changed")
		}
		if method == http.MethodGet && w.Body.String() != "2345" {
			t.Fatal("Range data changed")
		}
		if method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD wrote content")
		}
	}
}

type shortContentWriter struct{ *httptest.ResponseRecorder }

func (w shortContentWriter) Write(data []byte) (int, error) {
	return w.ResponseRecorder.Write(data[:len(data)/2])
}

func TestBoundedContentRejectsShortWriteWithoutLosingTheByteCount(t *testing.T) {
	underlying := httptest.NewRecorder()
	bounded := &contentResponseWriter{ResponseWriter: shortContentWriter{underlying}, ctx: t.Context()}
	n, err := bounded.Write([]byte("12345678"))
	if n != 4 || !errors.Is(err, io.ErrShortWrite) || underlying.Body.String() != "1234" {
		t.Fatalf("short write: n=%d error=%v body=%q", n, err, underlying.Body.String())
	}
}

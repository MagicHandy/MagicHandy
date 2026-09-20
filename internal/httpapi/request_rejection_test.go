package httpapi

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEarlyRejectionsDoNotDrainUnfinishedHTTP1Uploads(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	for _, test := range []struct {
		name, extra string
		status      int
	}{
		{"authentication", "", http.StatusUnauthorized},
		{"origin", "Origin: https://untrusted.invalid\r\n", http.StatusForbidden},
		{"forwarding", "X-Forwarded-For: 198.51.100.15\r\n", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertUnfinishedUploadReply(t, host.Listener.Addr().String(), test.extra, test.status)
		})
	}
}

func assertUnfinishedUploadReply(t *testing.T, address, extra string, status int) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(connection, "POST /api/motion/start HTTP/1.1\r\nHost: %s\r\n%sTransfer-Encoding: chunked\r\n\r\n", address, extra); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("rejection waited for the upload: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal("rejection was truncated", err)
	}
	if response.StatusCode != status || !response.Close {
		t.Fatalf("unfinished upload response: status=%d close=%t", response.StatusCode, response.Close)
	}
}

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

func TestCredentialUploadsHaveADeadlineAndReleaseLoginCapacity(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	connections := make([]net.Conn, 0, 2)
	for _, path := range []string{"/api/auth/login", "/api/auth/recover"} {
		connection, err := net.DialTimeout("tcp", host.Listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = connection.Close() }()
		if err := connection.SetDeadline(time.Now().Add(7 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: 500\r\n\r\n{", path, host.Listener.Addr()); err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	for _, connection := range connections {
		reply, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodPost})
		if err != nil {
			t.Fatal("credential upload retained its slot past the read deadline", err)
		}
		_, err = io.ReadAll(reply.Body)
		_ = reply.Body.Close()
		if err != nil || reply.StatusCode != http.StatusBadRequest || !reply.Close {
			t.Fatal("incomplete credential body did not end cleanly", err)
		}
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if s.requestAdmission.snapshot()[loginLane].Active == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("credential upload deadline did not release admission")
}

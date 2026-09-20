package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// finishUnreadBody lets HTTP/1 return an early reply without waiting for an
// unused upload. Closing the connection prevents remaining body bytes from
// becoming another request; the read deadline also bounds net/http's final
// body cleanup. HTTP/2 already separates request and response stream lifetimes.
func finishUnreadBody(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor != 1 || r.Body == nil || r.Body == http.NoBody {
		return
	}
	w.Header().Set("Connection", "close")
	_ = http.NewResponseController(w).SetReadDeadline(time.Now())
}

func rejectRequest(w http.ResponseWriter, r *http.Request, status int, err error) {
	finishUnreadBody(w, r)
	writeBoundedJSON(w, status, map[string]string{"error": err.Error()})
}

// writeBoundedJSON bounds early rejection/Stop replies outside the usual
// authenticated stream lifetime. Flush before clearing the socket deadline.
func writeBoundedJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := writeContextBytes(ctx, w, append(data, '\n')); err != nil {
		panic(http.ErrAbortHandler)
	}
}

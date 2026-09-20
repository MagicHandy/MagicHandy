package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

func writeContextBytes(ctx context.Context, w http.ResponseWriter, data []byte) error {
	_, err := writeContextChunk(ctx, w, data)
	return err
}

func writeContextChunk(ctx context.Context, w http.ResponseWriter, data []byte) (int, error) {
	controller := http.NewResponseController(w)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	interrupted := make(chan struct{})
	interrupt := context.AfterFunc(ctx, func() {
		defer close(interrupted)
		_ = controller.SetWriteDeadline(time.Now())
	})
	defer func() {
		if interrupt() {
			_ = controller.SetWriteDeadline(time.Time{})
		} else {
			<-interrupted
		}
	}()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	written, err := w.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, io.ErrShortWrite
	}
	if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return written, err
	}
	return written, ctx.Err()
}

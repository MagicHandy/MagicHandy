package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func writeContextBytes(ctx context.Context, w http.ResponseWriter, data []byte) error {
	controller := http.NewResponseController(w)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
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
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return ctx.Err()
}

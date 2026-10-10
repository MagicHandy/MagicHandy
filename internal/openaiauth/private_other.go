//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package openaiauth

import (
	"context"
	"errors"
)

func restrictPath(string) error {
	return errors.New("protected OpenAI credentials are unsupported on this host")
}
func lockCredentials(context.Context, string) (func(), error) {
	return nil, errors.New("protected OpenAI credentials are unsupported on this host")
}

//go:build !windows

package httpapi

import "context"

func systemHostPathPicker(context.Context, hostPathPickerSpec, string) (string, bool, error) {
	return "", false, errHostPathPickerUnsupported
}

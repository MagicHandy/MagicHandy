//go:build windows

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var hostPathPickerMu sync.Mutex

func systemHostPathPicker(ctx context.Context, spec hostPathPickerSpec, current string) (string, bool, error) {
	hostPathPickerMu.Lock()
	defer hostPathPickerMu.Unlock()
	if strings.ContainsRune(current, 0) {
		return "", false, errors.New("current path contains an invalid character")
	}
	const script = `
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
Add-Type -AssemblyName System.Windows.Forms
$current = $env:MAGICHANDY_PICKER_CURRENT
if ($env:MAGICHANDY_PICKER_DIRECTORY -eq '1') {
    $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
    $dialog.Description = $env:MAGICHANDY_PICKER_TITLE
    $dialog.ShowNewFolderButton = $false
    if ([System.IO.Directory]::Exists($current)) { $dialog.SelectedPath = $current }
} else {
    $dialog = New-Object System.Windows.Forms.OpenFileDialog
    $dialog.Title = $env:MAGICHANDY_PICKER_TITLE
    $dialog.Filter = $env:MAGICHANDY_PICKER_FILTER
    $dialog.CheckFileExists = $true
    $dialog.CheckPathExists = $true
    if ([System.IO.File]::Exists($current)) {
        $dialog.InitialDirectory = [System.IO.Path]::GetDirectoryName($current)
        $dialog.FileName = [System.IO.Path]::GetFileName($current)
    } elseif ([System.IO.Directory]::Exists($current)) {
        $dialog.InitialDirectory = $current
    } elseif (-not [string]::IsNullOrWhiteSpace($current)) {
        $parent = [System.IO.Path]::GetDirectoryName($current)
        if ([System.IO.Directory]::Exists($parent)) { $dialog.InitialDirectory = $parent }
    }
}
try {
    if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
        $selected = if ($env:MAGICHANDY_PICKER_DIRECTORY -eq '1') { $dialog.SelectedPath } else { $dialog.FileName }
        [Console]::Out.Write($selected)
    }
} finally {
    $dialog.Dispose()
}`
	// #nosec G204,G702 -- the executable and script are fixed; user paths are passed
	// only through environment variables and never interpolated into PowerShell.
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	directory := "0"
	if spec.Directory {
		directory = "1"
	}
	command.Env = append(os.Environ(),
		"MAGICHANDY_PICKER_CURRENT="+current,
		"MAGICHANDY_PICKER_DIRECTORY="+directory,
		"MAGICHANDY_PICKER_FILTER="+spec.Filter,
		"MAGICHANDY_PICKER_TITLE="+spec.Title,
	)
	output, err := command.Output()
	if err != nil {
		return "", false, fmt.Errorf("open Windows path picker: %w", err)
	}
	path := strings.TrimSpace(string(output))
	return path, path == "", nil
}

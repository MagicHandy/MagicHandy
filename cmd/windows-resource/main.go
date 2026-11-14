// Command windows-resource rebuilds the resource object that gives
// magichandy.exe its icon. Run it from the repository root after changing
// assets/brand/magichandy.ico:
//
//	go run ./cmd/windows-resource
//
// It is a developer tool and is not part of any release payload.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/winresource"
)

func main() {
	icon := flag.String("icon", "assets/brand/magichandy.ico", "Windows icon file to embed")
	output := flag.String("out", "cmd/magichandy/resource_windows_amd64.syso", "resource object to write")
	flag.Parse()

	if err := run(*icon, *output); err != nil {
		fmt.Fprintln(os.Stderr, "windows-resource:", err)
		os.Exit(1)
	}
}

func run(iconPath, outputPath string) error {
	icon, err := os.ReadFile(iconPath) // #nosec G304 -- developer tool reading a path the developer names
	if err != nil {
		return err
	}
	object, err := winresource.Object(icon)
	if err != nil {
		return fmt.Errorf("%s: %w", iconPath, err)
	}
	if err := os.WriteFile(outputPath, object, 0o644); err != nil { // #nosec G306 G703 -- developer tool writing a build input to a path the developer names
		return err
	}
	fmt.Printf("wrote %s (%d bytes) from %s\n", outputPath, len(object), iconPath)
	return nil
}

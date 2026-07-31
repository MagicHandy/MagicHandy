// Command import-curated-patterns loads generated .mhpattern.json files into the local library.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/patterns"
)

func main() {
	source := flag.String("source", "", "directory containing .mhpattern.json files")
	dataDir := flag.String("data-dir", "", "MagicHandy data directory")
	flag.Parse()

	if strings.TrimSpace(*source) == "" {
		fmt.Fprintln(os.Stderr, "source directory is required")
		os.Exit(2)
	}

	resolvedDataDir, err := config.ResolveDataDir(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		os.Exit(1)
	}
	store, err := config.OpenStore(resolvedDataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	library, err := patterns.OpenWithDatabase(store.Datastore())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pattern library: %v\n", err)
		os.Exit(1)
	}

	entries, err := os.ReadDir(*source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read source: %v\n", err)
		os.Exit(1)
	}

	imported := 0
	skipped := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".mhpattern.json") {
			continue
		}
		path := filepath.Join(*source, entry.Name())
		data, readErr := os.ReadFile(path) // #nosec G304 -- path is built from an explicit CLI source directory.
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", entry.Name(), readErr)
			skipped++
			continue
		}
		result, importErr := library.Import(entry.Name(), data, "pattern")
		if importErr != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", entry.Name(), importErr)
			skipped++
			continue
		}
		if result.Pattern != nil {
			fmt.Printf("imported %s (%s)\n", result.Pattern.Name, result.Pattern.ID)
			imported++
		}
	}

	fmt.Printf("done: imported=%d skipped=%d\n", imported, skipped)
	if imported == 0 {
		os.Exit(1)
	}
}

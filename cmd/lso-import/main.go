// Command lso-import copies library data from a legacy LSO app.sqlite into MagicHandy.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "lso-import: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("lso-import", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	lsoPath := flags.String("lso-db", "", "path to legacy LSO app.sqlite (required)")
	dataDir := flags.String("data-dir", "", "MagicHandy data directory (default: app data dir)")
	dryRun := flags.Bool("dry-run", false, "validate inputs without writing")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *lsoPath == "" {
		return fmt.Errorf("--lso-db is required")
	}

	resolvedDataDir, err := config.ResolveDataDir(*dataDir)
	if err != nil {
		return err
	}
	if *dryRun {
		abs, err := filepath.Abs(*lsoPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("LSO database: %w", err)
		}
		fmt.Printf("dry-run ok: would import %s -> %s\n", abs, filepath.Join(resolvedDataDir, store.DBFileName))
		return nil
	}

	db, err := store.Open(resolvedDataDir)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	result, err := store.ImportFromLSOWithOptions(db, *lsoPath, store.LSOImportOptions{
		LSODataDir: store.ResolveLSODataDir(*lsoPath),
		TargetDir:  resolvedDataDir,
	})
	if err != nil {
		return err
	}

	fmt.Printf(
		"imported personas=%d funscript_files=%d motion_blocks=%d saved_queues=%d persona_media_files=%d active_persona_id=%q\n",
		result.Personas,
		result.FunscriptFiles,
		result.MotionBlocks,
		result.SavedQueues,
		result.PersonaMediaFiles,
		result.ActivePersonaID,
	)
	return nil
}

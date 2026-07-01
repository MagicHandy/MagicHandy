// Command retarget-validate runs the Phase 7 live-device retarget checklist.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/validation"
)

const (
	defaultConnectionKeyEnv = "MAGICHANDY_HANDY_CONNECTION_KEY"
	defaultApplicationIDEnv = "MAGICHANDY_API_V3_APPLICATION_ID"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		connectionKeyEnv   string
		connectionKeyStdin bool
		applicationID      string
		baseURL            string
		outputDir          string
		maxSpeed           int
		settle             time.Duration
		timeout            time.Duration
	)
	flag.StringVar(&connectionKeyEnv, "connection-key-env", defaultConnectionKeyEnv, "environment variable containing the private Handy connection key")
	flag.BoolVar(&connectionKeyStdin, "connection-key-stdin", false, "read the private Handy connection key from stdin")
	flag.StringVar(&applicationID, "app-id", envOrDefault(defaultApplicationIDEnv, config.BundledAPIApplicationID), "public Handy API v3 application ID")
	flag.StringVar(&baseURL, "base-url", "", "Handy API v3 base URL override")
	flag.StringVar(&outputDir, "out", defaultOutputDir(), "directory for exported trace JSON files")
	flag.IntVar(&maxSpeed, "max-speed", validation.DefaultValidationMaxSpeedPercent, "maximum automated validation speed percent")
	flag.DurationVar(&settle, "settle", 1500*time.Millisecond, "settle time between validation changes")
	flag.DurationVar(&timeout, "timeout", 90*time.Second, "overall validation timeout")
	flag.Parse()

	connectionKey, err := resolveConnectionKey(connectionKeyEnv, connectionKeyStdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		fmt.Fprintln(os.Stderr, "public Handy API v3 application ID is required")
		return 2
	}
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve output directory: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cloud, err := transport.NewCloudRESTTransport(
		transport.CloudPrerequisites{
			ApplicationID: applicationID,
			ConnectionKey: connectionKey,
			FirmwareMajor: 4,
			APIMajor:      3,
			HSPAvailable:  true,
		},
		transport.CloudBuildOptions{},
		transport.CloudEndpointConfig{BaseURL: baseURL},
		&http.Client{Timeout: 10 * time.Second},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create Cloud REST transport: %v\n", err)
		return 2
	}
	if check, err := cloud.CheckConnection(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Cloud REST connection check failed: %v\n", err)
		writeSafeJSON(os.Stderr, check)
		return 1
	}

	traces := diagnostics.NewTraceRing(2048)
	engine, err := motion.NewEngine(motion.EngineOptions{
		Transport:        cloud,
		Traces:           traces,
		ChunkSize:        6,
		SampleInterval:   150 * time.Millisecond,
		DispatchInterval: 250 * time.Millisecond,
		StreamIDPrefix:   "phase7",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create motion engine: %v\n", err)
		return 2
	}

	result, err := validation.RunRetargetValidation(ctx, engine, traces, validation.RetargetOptions{
		ExportDir:       absOutputDir,
		MaxSpeedPercent: maxSpeed,
		SettlingDelay:   settle,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "retarget validation failed: %v\n", err)
		writeSafeJSON(os.Stderr, result)
		return 1
	}
	writeSafeJSON(os.Stdout, result)
	return 0
}

func resolveConnectionKey(envName string, readStdin bool) (string, error) {
	if readStdin {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if err != nil {
			return "", fmt.Errorf("read connection key from stdin: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", errorsForMissingConnectionKey(envName)
		}
		return key, nil
	}
	key := strings.TrimSpace(os.Getenv(envName))
	if key == "" {
		return "", errorsForMissingConnectionKey(envName)
	}
	return key, nil
}

func errorsForMissingConnectionKey(envName string) error {
	return fmt.Errorf("private Handy connection key is required; set %s or pass -connection-key-stdin", envName)
}

func envOrDefault(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func defaultOutputDir() string {
	return filepath.Join("traces", "phase7-retarget-"+time.Now().UTC().Format("20060102-150405"))
}

func writeSafeJSON(writer io.Writer, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(writer, "%+v\n", value)
		return
	}
	_, _ = writer.Write(append(data, '\n'))
}

package httpapi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVoiceInstallFailureProcess(_ *testing.T) {
	if os.Getenv("MAGICHANDY_TEST_INSTALL_FAILURE") == "1" {
		os.Exit(73)
	}
}

func voiceInstallExitError(t *testing.T) error {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestVoiceInstallFailureProcess$") // #nosec G204 -- current test binary with a fixed helper-test selector.
	command.Env = append(os.Environ(), "MAGICHANDY_TEST_INSTALL_FAILURE=1")
	err = command.Run()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 73 {
		t.Fatalf("fixture did not exit with its diagnostic code: %v", err)
	}
	return err
}

func TestVoiceInstallFailureAcceptsOnlyKnownStagesForTheCurrentFailedJob(t *testing.T) {
	exitError := voiceInstallExitError(t)
	marker := `MAGICHANDY_SETUP_FAILURE:{"stage":"Chatterbox dependency installation","exit_code":73}`
	for _, test := range []struct {
		name, id, output string
		err              error
		want             string
	}{
		{"stage", "active", "\r\n" + marker + "\r\nPowerShell error footer", exitError, "Chatterbox dependency installation failed (exit 73)"},
		{"latest stage", "active", marker + "\n" + `MAGICHANDY_SETUP_FAILURE:{"stage":"Python voice runtime verification","exit_code":2}`, exitError, "Python voice runtime verification failed (exit 2)"},
		{"wrong job", "stale", marker, exitError, ""},
		{"successful process", "active", marker, nil, ""},
		{"cancellation", "active", marker, context.Canceled, ""},
		{"ordinary stderr", "active", "secret-token: private native diagnostic", exitError, ""},
		{"malformed marker", "active", "MAGICHANDY_SETUP_FAILURE:{", exitError, ""},
		{"untrusted label", "active", `MAGICHANDY_SETUP_FAILURE:{"stage":"secret-token","exit_code":73}`, exitError, ""},
		{"zero exit", "active", `MAGICHANDY_SETUP_FAILURE:{"stage":"Python environment creation","exit_code":0}`, exitError, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &setupManager{job: &setupJobState{setupJob: setupJob{ID: "active", Output: test.output}}}
			got := manager.voiceInstallFailure(test.id, test.err)
			if test.want == "" {
				if got != test.err {
					t.Fatalf("unexpected failure enrichment: %v", got)
				}
			} else if got == nil || !strings.Contains(got.Error(), test.want) || !errors.Is(got, test.err) {
				t.Fatalf("failure = %v, want %q with original cause", got, test.want)
			}
		})
	}
}

func TestVoiceInstallFailureSurvivesRestartWithoutPersistingRawOutput(t *testing.T) {
	server := newTestServer(t)
	ctx, job, err := server.setup.reserveJob("voice", "chatterbox", "cpu", "queued")
	if err != nil {
		t.Fatal(err)
	}
	writer := &setupOutputWriter{manager: server.setup, id: job.ID}
	for _, piece := range []string{
		"private-installer-output\nMAGICHANDY_SETUP_FAIL",
		"URE:{\"stage\":\"Pinned Chatterbox engine installation\",\"exit_code\":73}\r\n",
		"unrelated PowerShell footer\n",
	} {
		_, _ = writer.Write([]byte(piece))
	}
	err = server.setup.voiceInstallFailure(job.ID, voiceInstallExitError(t))
	server.setup.finishJob(ctx, job.ID, err, "Chatterbox Turbo")
	data, err := os.ReadFile(server.setup.setupResultPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-installer-output") || strings.Contains(string(data), "PowerShell footer") {
		t.Fatal("raw installer output was persisted")
	}
	reloaded := &setupManager{dataDir: server.setup.dataDir}
	reloaded.loadPersistedSetupJob()
	result := reloaded.Snapshot()
	if result == nil || result.Status != setupJobFailed || result.Output != "" ||
		!strings.Contains(result.Message, "Pinned Chatterbox engine installation failed (exit 73)") {
		t.Fatalf("restored failure lost its operation: %+v", result)
	}
}

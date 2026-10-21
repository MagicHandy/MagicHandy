package httpapi

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/startup"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var diagnosticWebSocketURL = regexp.MustCompile(`(?i)\bwss?://[^\s'"<>]+`)

type diagnosticReportOptions struct {
	Settings    bool   `json:"settings"`
	Traces      bool   `json:"traces"`
	Chat        string `json:"chat"`
	ModelOutput bool   `json:"model_output"`
	Personas    bool   `json:"personas"`
	Prompts     bool   `json:"prompts"`
	Memories    bool   `json:"memories"`
}

func diagnosticOptions(query url.Values) (diagnosticReportOptions, error) {
	options := diagnosticReportOptions{Settings: true, Traces: true, Chat: "failures"}
	flags := map[string]*bool{"settings": &options.Settings, "traces": &options.Traces,
		"model_output": &options.ModelOutput, "personas": &options.Personas, "prompts": &options.Prompts, "memories": &options.Memories}
	for key, values := range query {
		if len(values) != 1 {
			return options, errors.New("report options must have exactly one value")
		}
		if key == "chat" {
			if values[0] != "none" && values[0] != "failures" && values[0] != "full" {
				return options, errors.New("chat must be none, failures or full")
			}
			options.Chat = values[0]
			continue
		}
		flag, ok := flags[key]
		if !ok || (values[0] != "true" && values[0] != "false") {
			return options, errors.New("unknown or invalid diagnostic report option")
		}
		*flag = values[0] == "true"
	}
	if options.Chat == "none" {
		options.ModelOutput = false
	}
	return options, nil
}

func (s *Server) handleDiagnosticReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New(administratorHostAccessRequired))
		return
	}
	options, err := diagnosticOptions(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	redact, err := s.diagnosticReportRedactor(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("credentials could not be redacted; no report was exported"))
		return
	}
	// Complete and validate the archive before sending headers. Failures never
	// masquerade as successful partial downloads. The private temporary file is
	// removed after serving; no report is retained in the application's data.
	file, err := os.CreateTemp("", "magichandy-diagnostics-*.zip")
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("diagnostic report could not be prepared"))
		return
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	archive := &diagnosticArchive{zip: zip.NewWriter(file), ctx: ctx, redact: redact, remaining: diagnosticReportMaxBytes}
	generated := time.Now().UTC()
	err = s.populateDiagnosticReport(r.WithContext(ctx), archive, options, generated)
	closeErr := archive.zip.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		if errors.Is(err, errDiagnosticReportTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
		} else {
			writeError(w, http.StatusInternalServerError, errors.New("diagnostic report could not be completed; retry or exclude optional content"))
		}
		return
	}
	if _, err := file.Seek(0, 0); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("diagnostic report could not be read"))
		return
	}
	filename := "magichandy-diagnostics-" + generated.Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeContent(w, r, filename, generated, file)
}

func (s *Server) diagnosticReportRedactor(ctx context.Context) (func(string) string, error) {
	common := s.setup.setupReportRedactor()
	locations := func(value string) string {
		return diagnosticWebSocketURL.ReplaceAllString(common(value), "[redacted URL]")
	}
	if s.cloudPlanning.auth == nil {
		return locations, nil
	}
	cloud, err := s.cloudPlanning.auth.DiagnosticRedactor(ctx)
	if err != nil {
		return nil, err
	}
	return func(value string) string { return locations(cloud(value)) }, nil
}

func (s *Server) populateDiagnosticReport(r *http.Request, archive *diagnosticArchive, options diagnosticReportOptions, generated time.Time) error {
	if err := archive.addJSON("status.json", s.diagnosticStatus(r)); err != nil {
		return err
	}
	if options.Settings {
		settings, status := s.store.PublicSnapshot()
		// Freeform personalization is an explicit inclusion, even when it is
		// stored in settings instead of the persona or prompt-set libraries.
		settings.LLM.PersonaDescription, settings.LLM.CustomAnatomy = "", ""
		settings.Voice.TTSReferenceText, settings.Voice.TTSTonePrompt = "", ""
		if err := archive.addJSON("settings.json", map[string]any{"settings": settings, "load_status": status}); err != nil {
			return err
		}
	}
	if options.Traces {
		if err := archive.addJSON("motion/live-trace.json", s.currentTraceExport()); err != nil {
			return err
		}
		last, ok, err := s.loadLastMotionTrace(r.Context())
		if err != nil {
			return err
		}
		if ok {
			if err := archive.addJSON("motion/last-stopped-trace.json", last); err != nil {
				return err
			}
		} else {
			archive.notes = append(archive.notes, "No stopped motion trace has been retained yet.")
		}
	}
	if err := s.addDiagnosticPersonalization(r.Context(), archive, options); err != nil {
		return err
	}
	if options.Chat != "none" {
		if err := s.addDiagnosticFailures(r.Context(), archive, options); err != nil {
			return err
		}
	}
	if options.Chat == "full" {
		if err := s.addDiagnosticChat(r.Context(), archive); err != nil {
			return err
		}
	}
	archive.notes = append(archive.notes,
		"Status, including retained startup diagnostics, is always included. Only selected sections are exported. No model or device request is made.",
		fmt.Sprintf("Chat exports contain all retained committed messages at each session's captured head (up to %d per session); pending, pruned and deleted messages are not recoverable. Recent failures are retained separately (up to %d across chats).", chat.MessageRetentionCap, chat.FailureRetention),
		"Prompt inclusion contains captured system prompts, which can also contain conversation, persona, lore or memory text. Failure content may have been truncated when captured.",
		"Stored credentials, common token forms, URLs and local paths are redacted. Review optional personal content before sharing. No audio, images, environment dump, account sessions or database file is included.",
		"Files ending in .jsonl contain one complete message as JSON per line. Session file names are listed in chat/sessions.json.")
	return archive.addJSON("manifest.json", map[string]any{"schema_version": "magichandy.diagnostics.v1", "generated_at": generated.Format(time.RFC3339Nano),
		"selection": options, "files": append(append([]string{}, archive.files...), "manifest.json"), "notes": archive.notes})
}

func (s *Server) diagnosticStatus(r *http.Request) map[string]any {
	mode := s.modes.Status()
	mode.LastSay = ""
	intiface := s.intifaceSnapshot()
	intiface.Status.RecentDispatches = nil
	intiface.Diagnostics = diagnosticTransportSummary(intiface.Diagnostics)
	_, tools := s.mediaTools(r.Context())
	return map[string]any{
		"app_version": s.version.Version, "app_commit": s.version.Commit, "platform": runtime.GOOS + "/" + runtime.GOARCH,
		"startup":    startup.ReadReports(s.store.DataDir()),
		"go_version": runtime.Version(), "cpu_threads": runtime.NumCPU(), "uptime_seconds": int64(time.Since(s.started).Seconds()),
		"browser_user_agent": r.UserAgent(), "stop_sequence": s.stopSequence.Load(), "motion_simulated": s.motion.simulated,
		"motion": s.clientMotionState(r), "modes": mode, "llm": s.llmState(r.Context()), "voice_workers": s.voice.Status(),
		"transport": diagnosticTransportSummary(s.transport.Diagnostics()), "cloud_transport": diagnosticTransportSummary(s.cloudDiagnostics()), "bluetooth_transport": diagnosticTransportSummary(s.bluetoothDiagnostics()),
		"native_bluetooth_transport": s.nativeBluetoothSnapshot(),
		"intiface_transport":         intiface, "media_tools": tools, "trace_summary": s.traces.Summary(),
	}
}

func diagnosticTransportSummary(status transport.TransportDiagnostics) transport.TransportDiagnostics {
	status.LastCommand = nil // Point payloads belong to the optional trace files.
	return status
}

func (s *Server) addDiagnosticChat(ctx context.Context, archive *diagnosticArchive) error {
	sessions, err := s.chatLog.SessionsContext(ctx)
	if err != nil {
		return err
	}
	var index []map[string]any
	for i, session := range sessions {
		name := "chat/history-" + strconv.Itoa(i+1) + ".jsonl"
		writer, err := archive.entry(name)
		if err != nil {
			return err
		}
		count, after := 0, int64(0)
		for after < session.LatestSeq {
			page, err := s.chatLog.AfterSessionContext(ctx, session.ID, after, 25)
			if err != nil {
				return err
			}
			if len(page) == 0 {
				break
			}
			for _, message := range page {
				after = message.Seq
				if after > session.LatestSeq {
					break
				}
				message.ClientID = ""
				if err := archive.writeJSON(writer, message); err != nil {
					return err
				}
				count++
			}
		}
		index = append(index, map[string]any{"session": session, "file": name, "exported_messages": count})
		if count != session.MessageCount {
			archive.notes = append(archive.notes, fmt.Sprintf("%s changed during export: captured %d of %d messages present when listed.", name, count, session.MessageCount))
		}
	}
	return archive.addJSON("chat/sessions.json", index)
}

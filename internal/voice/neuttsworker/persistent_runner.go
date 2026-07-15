package neuttsworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	persistentRunnerMarker  = "magichandy_neutts_stream_v1"
	persistentFrameReady    = byte(1)
	persistentFrameAudio    = byte(2)
	persistentFrameDone     = byte(3)
	persistentFrameError    = byte(4)
	persistentFrameCanceled = byte(5)
	persistentFrameMax      = maxPCMBytes
	persistentStopGrace     = 500 * time.Millisecond
)

var persistentFrameMagic = [4]byte{'M', 'H', 'T', 'S'}

type persistentFrame struct {
	kind    byte
	payload []byte
}

type persistentCommand struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

type persistentRunner struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *tailBuffer

	writeMu   sync.Mutex
	requestMu sync.Mutex
	stateMu   sync.Mutex
	waitErr   error
	done      chan struct{}
	stopOnce  sync.Once
}

func detectRunnerMode(ctx context.Context, options Options) (bool, error) {
	// #nosec G204 -- executable and arguments are explicit local settings,
	// invoked directly without shell expansion.
	command := exec.CommandContext(ctx, options.RunnerPath, "--help")
	command.Dir = runnerWorkingDirectory(options.RunnerPath)
	command.Env = offlineEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return false, errors.New("NeuTTS runner compatibility check timed out")
		}
		message := "NeuTTS runner compatibility check failed"
		if detail := string(bytes.TrimSpace(output)); detail != "" {
			message += ": " + detail
		}
		return false, errors.New(message)
	}
	help := bytes.ToLower(output)
	if !bytes.Contains(help, []byte("--codes")) || !bytes.Contains(help, []byte("--ref-text")) {
		return false, errors.New("NeuTTS runner is incompatible: --help did not advertise the required reference-code options")
	}
	if bytes.Contains(help, []byte(persistentRunnerMarker)) && bytes.Contains(help, []byte("--serve")) {
		return true, nil
	}
	if !bytes.Contains(help, []byte("stream_pcm")) {
		return false, errors.New("NeuTTS runner is incompatible: expected stream_pcm or MagicHandy's persistent protocol")
	}
	return false, nil
}

func startPersistentRunner(ctx context.Context, options Options) (*persistentRunner, error) {
	args := runnerServerArgs(options)
	// #nosec G204 -- executable and arguments are explicit local settings,
	// invoked directly without shell expansion.
	command := exec.Command(options.RunnerPath, args...)
	command.Dir = runnerWorkingDirectory(options.RunnerPath)
	command.Env = offlineEnvironment()
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open persistent NeuTTS stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open persistent NeuTTS stdout: %w", err)
	}
	stderr := &tailBuffer{}
	command.Stderr = stderr
	if err = command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start persistent NeuTTS runner: %w", err)
	}
	runner := &persistentRunner{
		command: command,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdout, 64*1024),
		stderr:  stderr,
		done:    make(chan struct{}),
	}
	go func() {
		err := command.Wait()
		runner.stateMu.Lock()
		runner.waitErr = err
		runner.stateMu.Unlock()
		close(runner.done)
	}()

	frameResult := make(chan struct {
		frame persistentFrame
		err   error
	}, 1)
	go func() {
		frame, readErr := runner.readReadyFrame()
		frameResult <- struct {
			frame persistentFrame
			err   error
		}{frame: frame, err: readErr}
	}()
	select {
	case <-ctx.Done():
		runner.Stop()
		return nil, fmt.Errorf("load persistent NeuTTS runner: %w", ctx.Err())
	case <-runner.done:
		return nil, runner.exitError("persistent NeuTTS runner exited during load")
	case result := <-frameResult:
		if result.err != nil {
			runner.Stop()
			return nil, runner.decorateError("persistent NeuTTS runner readiness failed", result.err)
		}
		if result.frame.kind != persistentFrameReady {
			runner.Stop()
			return nil, fmt.Errorf("persistent NeuTTS runner returned frame %d before ready", result.frame.kind)
		}
		var ready struct {
			Protocol int    `json:"protocol"`
			Codec    string `json:"codec"`
		}
		if err := json.Unmarshal(result.frame.payload, &ready); err != nil || ready.Protocol != 1 || ready.Codec == "" {
			runner.Stop()
			return nil, errors.New("persistent NeuTTS runner returned invalid readiness metadata")
		}
		return runner, nil
	}
}

func (r *persistentRunner) readReadyFrame() (persistentFrame, error) {
	for diagnostics := 0; ; diagnostics++ {
		prefix, err := r.stdout.Peek(len(persistentFrameMagic))
		if err != nil {
			return persistentFrame{}, fmt.Errorf("inspect persistent NeuTTS startup: %w", err)
		}
		if bytes.Equal(prefix, persistentFrameMagic[:]) {
			return r.readFrame()
		}
		if diagnostics >= 4 {
			return persistentFrame{}, errors.New("persistent NeuTTS runner returned too many stdout diagnostics")
		}
		line, err := r.stdout.ReadString('\n')
		if err != nil {
			return persistentFrame{}, errors.New("persistent NeuTTS runner returned a malformed startup diagnostic")
		}
		line = string(bytes.TrimSpace([]byte(line)))
		if !strings.HasPrefix(line, runnerDiagnostic) && !strings.HasPrefix(line, "NeuCodec: ") {
			return persistentFrame{}, fmt.Errorf("persistent NeuTTS runner returned unexpected stdout before readiness: %q", line)
		}
	}
}

func (r *persistentRunner) Speak(ctx context.Context, id, text string, onAudio func([]byte) error) error {
	r.requestMu.Lock()
	defer r.requestMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.Alive() {
		return r.exitError("persistent NeuTTS runner is not running")
	}
	if err := r.send(persistentCommand{Type: "speak", ID: id, Text: text}); err != nil {
		return r.decorateError("send persistent NeuTTS request", err)
	}

	requestDone := make(chan struct{})
	defer close(requestDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = r.send(persistentCommand{Type: "cancel", ID: id})
		case <-requestDone:
		}
	}()

	var callbackErr error
	for {
		frame, err := r.readFrame()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return r.decorateError("read persistent NeuTTS response", err)
		}
		switch frame.kind {
		case persistentFrameAudio:
			if callbackErr == nil {
				callbackErr = onAudio(frame.payload)
				if callbackErr != nil {
					_ = r.send(persistentCommand{Type: "cancel", ID: id})
				}
			}
		case persistentFrameDone:
			return callbackErr
		case persistentFrameCanceled:
			if callbackErr != nil {
				return callbackErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return context.Canceled
		case persistentFrameError:
			message := string(bytes.TrimSpace(frame.payload))
			if message == "" {
				message = "persistent NeuTTS synthesis failed"
			}
			return errors.New(message)
		default:
			return fmt.Errorf("persistent NeuTTS runner returned unknown frame %d", frame.kind)
		}
	}
}

func (r *persistentRunner) Alive() bool {
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (r *persistentRunner) Stop() {
	r.stopOnce.Do(func() {
		_ = r.send(persistentCommand{Type: "shutdown"})
		select {
		case <-r.done:
			return
		case <-time.After(persistentStopGrace):
		}
		if r.command.Process != nil {
			_ = r.command.Process.Kill()
		}
		<-r.done
	})
}

func (r *persistentRunner) send(command persistentCommand) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	data, err := json.Marshal(command)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = r.stdin.Write(data)
	return err
}

func (r *persistentRunner) readFrame() (persistentFrame, error) {
	header := make([]byte, 9)
	if _, err := io.ReadFull(r.stdout, header); err != nil {
		return persistentFrame{}, err
	}
	if !bytes.Equal(header[:4], persistentFrameMagic[:]) {
		return persistentFrame{}, errors.New("persistent NeuTTS runner returned invalid frame magic")
	}
	length := binary.LittleEndian.Uint32(header[5:9])
	if length > persistentFrameMax {
		return persistentFrame{}, fmt.Errorf("persistent NeuTTS runner frame exceeds %d bytes", persistentFrameMax)
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r.stdout, payload); err != nil {
		return persistentFrame{}, err
	}
	return persistentFrame{kind: header[4], payload: payload}, nil
}

func (r *persistentRunner) exitError(prefix string) error {
	r.stateMu.Lock()
	waitErr := r.waitErr
	r.stateMu.Unlock()
	if waitErr != nil {
		return r.decorateError(prefix, waitErr)
	}
	return r.decorateError(prefix, nil)
}

func (r *persistentRunner) decorateError(prefix string, err error) error {
	message := prefix
	if err != nil {
		message += ": " + err.Error()
	}
	if detail := string(bytes.TrimSpace([]byte(r.stderr.String()))); detail != "" {
		message += ": " + detail
	}
	return errors.New(message)
}

func runnerServerArgs(options Options) []string {
	args := []string{
		"--serve",
		"--codes", options.ReferenceCodes,
		"--ref-text", options.ReferenceText,
		"--chunk", fmt.Sprintf("%d", options.ChunkTokens),
	}
	if options.Backbone != "" {
		args = append(args, "--backbone", options.Backbone)
	}
	if options.GGUFFile != "" {
		args = append(args, "--gguf-file", options.GGUFFile)
	}
	return args
}

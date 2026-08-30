package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	parakeetRunnerURL     = "https://github.com/mudler/parakeet.cpp/releases/download/v0.4.0/parakeet-v0.4.0-bin-win-cpu-x64.zip"
	parakeetRunnerSHA     = "2880150a1bad2944baed46f2e6bb9f1bc55263a9f2bb85573785a7ec4fa35f27"
	parakeetModelRevision = "bf0af9f425fa01809cadec671b3cb672709d13e9"
	parakeetModelURL      = "https://huggingface.co/mudler/parakeet-cpp-gguf/resolve/" + parakeetModelRevision + "/tdt-0.6b-v3-q4_k.gguf?download=true"
	parakeetModelSHA      = "993d73feb4206dadda865ab25bd64b50c48dc4d013c3bf6126a721f28b1d5ee8"

	parakeetInstallSpaceFloor = uint64(750 << 20)
	parakeetInstallSpaceSlack = uint64(64 << 20)
	parakeetRunnerMaxBytes    = int64(32 << 20)
	parakeetModelMaxBytes     = int64(1 << 30)
	parakeetDownloadAttempts  = 3
)

type parakeetDownloadAsset struct {
	label       string
	url         string
	destination string
	sha256      string
	maxBytes    int64
}

var errParakeetRangeComplete = errors.New("saved partial already contains the complete response")

func parakeetInstallPaths(dataDir string) setupParakeetInstallResult {
	return setupParakeetInstallResult{
		ServerPath: filepath.Join(dataDir, "voice", "parakeet", "runner", "parakeet-server.exe"),
		ModelPath:  filepath.Join(dataDir, "voice", "parakeet", "tdt-0.6b-v3-q4_k.gguf"),
	}
}

func parakeetDownloadAssets(dataDir string) []parakeetDownloadAsset {
	root := filepath.Join(dataDir, "voice", "parakeet")
	return []parakeetDownloadAsset{
		{
			label:       "Parakeet runner",
			url:         parakeetRunnerURL,
			destination: filepath.Join(root, "parakeet-v0.4.0-bin-win-cpu-x64.zip"),
			sha256:      parakeetRunnerSHA,
			maxBytes:    parakeetRunnerMaxBytes,
		},
		{
			label:       "Parakeet model",
			url:         parakeetModelURL,
			destination: filepath.Join(root, "tdt-0.6b-v3-q4_k.gguf"),
			sha256:      parakeetModelSHA,
			maxBytes:    parakeetModelMaxBytes,
		},
	}
}

func parakeetPartialPath(asset parakeetDownloadAsset) string {
	return asset.destination + "." + asset.sha256 + ".partial"
}

func inspectParakeetPartial(dataDir string) (int64, bool) {
	var bytes int64
	found := false
	for _, asset := range parakeetDownloadAssets(dataDir) {
		if info, err := os.Stat(parakeetPartialPath(asset)); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			bytes += info.Size()
			found = true
		}
	}
	return bytes, found
}

func preflightParakeetInstall(dataDir string) error {
	root := filepath.Join(dataDir, "voice", "parakeet")
	if err := os.MkdirAll(root, 0o750); err != nil {
		return fmt.Errorf("prepare Parakeet module directory: %w", err)
	}
	probe, err := os.CreateTemp(root, ".write-check-*")
	if err != nil {
		return fmt.Errorf("managed Parakeet module directory is not writable: %w", err)
	}
	probeName := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probeName)
		return fmt.Errorf("verify Parakeet module directory: %w", closeErr)
	}
	if err := os.Remove(probeName); err != nil {
		return fmt.Errorf("clean up Parakeet write check: %w", err)
	}

	var reusable uint64
	for _, asset := range parakeetDownloadAssets(dataDir) {
		for _, candidate := range []string{asset.destination, parakeetPartialPath(asset)} {
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
				reusable += uint64(info.Size()) //nolint:gosec // The positive-size guard makes this conversion safe.
			}
		}
	}
	if reusable > parakeetInstallSpaceFloor-parakeetInstallSpaceSlack {
		reusable = parakeetInstallSpaceFloor - parakeetInstallSpaceSlack
	}
	required := parakeetInstallSpaceFloor - reusable
	available, err := setupAvailableBytes(root)
	if err != nil {
		return fmt.Errorf("check free space for Parakeet: %w", err)
	}
	if available < required {
		return fmt.Errorf(
			"not enough free space for Parakeet: %d MiB available, about %d MiB still required",
			available/(1<<20), required/(1<<20),
		)
	}
	return nil
}

func (m *setupManager) downloadParakeetAssets(ctx context.Context, id string) error {
	for _, asset := range parakeetDownloadAssets(m.dataDir) {
		if err := m.downloadVerifiedParakeetAsset(ctx, id, asset); err != nil {
			return err
		}
	}
	return nil
}

func (m *setupManager) downloadVerifiedParakeetAsset(ctx context.Context, id string, asset parakeetDownloadAsset) error {
	if match, err := fileMatchesSHA256(asset.destination, asset.sha256); err == nil && match {
		m.updateJob(id, setupJobRunning, asset.label+" already downloaded and verified.", "")
		return nil
	}
	if err := os.Remove(asset.destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace unverified %s: %w", strings.ToLower(asset.label), err)
	}
	if err := os.MkdirAll(filepath.Dir(asset.destination), 0o750); err != nil {
		return fmt.Errorf("prepare %s download: %w", strings.ToLower(asset.label), err)
	}

	partial := parakeetPartialPath(asset)
	var lastErr error
	for attempt := 1; attempt <= parakeetDownloadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = m.downloadParakeetAssetAttempt(ctx, id, asset, partial)
		if errors.Is(lastErr, errParakeetRangeComplete) {
			match, hashErr := fileMatchesSHA256(partial, asset.sha256)
			if hashErr == nil && match {
				lastErr = nil
			} else {
				_ = os.Remove(partial)
				lastErr = errors.New("saved partial did not match the expected SHA-256")
			}
		}
		if lastErr == nil {
			match, hashErr := fileMatchesSHA256(partial, asset.sha256)
			if hashErr != nil {
				lastErr = hashErr
			} else if !match {
				_ = os.Remove(partial)
				lastErr = errors.New("SHA-256 verification failed")
			}
		}
		if lastErr == nil {
			if err := os.Rename(partial, asset.destination); err != nil {
				return fmt.Errorf("activate verified %s: %w", strings.ToLower(asset.label), err)
			}
			m.updateJob(id, setupJobRunning, asset.label+" downloaded and verified.", "")
			return nil
		}
		if attempt < parakeetDownloadAttempts {
			m.updateJob(id, setupJobRunning, fmt.Sprintf("%s download interrupted; retry %d of %d will resume.", asset.label, attempt+1, parakeetDownloadAttempts), "")
			timer := time.NewTimer(time.Duration(attempt) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("download %s after %d attempts: %w; partial data was kept for resume", strings.ToLower(asset.label), parakeetDownloadAttempts, lastErr)
}

func (m *setupManager) downloadParakeetAssetAttempt(
	ctx context.Context,
	id string,
	asset parakeetDownloadAsset,
	partial string,
) error {
	offset := int64(0)
	if info, err := os.Stat(partial); err == nil && info.Mode().IsRegular() {
		offset = info.Size()
		if offset > asset.maxBytes {
			if err := os.Remove(partial); err != nil {
				return fmt.Errorf("discard oversized partial: %w", err)
			}
			offset = 0
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "MagicHandy/Parakeet-Installer")
	request.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 60 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	offset, appendPartial, err := parakeetResponseLayout(response, offset)
	if err != nil {
		return err
	}
	return m.writeParakeetResponse(id, asset, partial, response, offset, appendPartial)
}

func parakeetResponseLayout(response *http.Response, offset int64) (int64, bool, error) {
	switch response.StatusCode {
	case http.StatusOK:
		return 0, false, nil
	case http.StatusPartialContent:
		return offset, offset > 0, nil
	case http.StatusRequestedRangeNotSatisfiable:
		if offset > 0 {
			return 0, false, errParakeetRangeComplete
		}
		return 0, false, fmt.Errorf("download server rejected an empty range: HTTP %d", response.StatusCode)
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return 0, false, fmt.Errorf("download server returned HTTP %d", response.StatusCode)
	}
}

func (m *setupManager) writeParakeetResponse(
	id string,
	asset parakeetDownloadAsset,
	partial string,
	response *http.Response,
	offset int64,
	appendPartial bool,
) error {
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendPartial {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	output, err := os.OpenFile(partial, flags, 0o600) // #nosec G304 -- fixed app-owned destination.
	if err != nil {
		return err
	}
	completed := offset
	total := response.ContentLength
	if appendPartial && total >= 0 {
		total += offset
	}
	if contentTotal := contentRangeTotal(response.Header.Get("Content-Range")); contentTotal > 0 {
		total = contentTotal
	}
	m.updateJobProgress(id, asset.label, completed, total)

	buffer := make([]byte, 1<<20)
	lastProgress := time.Now()
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if completed+int64(count) > asset.maxBytes {
				_ = output.Close()
				_ = os.Remove(partial)
				return fmt.Errorf("%s exceeded the maximum accepted size", asset.label)
			}
			written, writeErr := output.Write(buffer[:count])
			completed += int64(written)
			if writeErr != nil {
				_ = output.Close()
				return writeErr
			}
			if written != count {
				_ = output.Close()
				return io.ErrShortWrite
			}
			if time.Since(lastProgress) >= 500*time.Millisecond {
				m.updateJobProgress(id, asset.label, completed, total)
				lastProgress = time.Now()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = output.Close()
			return readErr
		}
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	m.updateJobProgress(id, asset.label, completed, total)
	return nil
}

func contentRangeTotal(value string) int64 {
	separator := strings.LastIndex(value, "/")
	if separator < 0 || separator == len(value)-1 || value[separator+1:] == "*" {
		return 0
	}
	total, err := strconv.ParseInt(value[separator+1:], 10, 64)
	if err != nil || total <= 0 {
		return 0
	}
	return total
}

func fileMatchesSHA256(path, expected string) (bool, error) {
	file, err := os.Open(path) // #nosec G304 -- caller supplies an app-owned setup path.
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected), nil
}

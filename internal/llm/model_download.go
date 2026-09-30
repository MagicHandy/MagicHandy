package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// ImportStatusDownloading identifies an active verified catalog download.
	ImportStatusDownloading = "downloading"

	catalogDownloadAttempts   = 3
	catalogDownloadIdleLimit  = 90 * time.Second
	catalogProgressInterval   = 250 * time.Millisecond
	catalogDownloadUserAgent  = "MagicHandy/Model-Download"
	catalogDownloadBufferSize = 1 << 20
)

var (
	errCatalogChecksum      = errors.New("the downloaded model did not match its pinned SHA-256 and was discarded")
	errCatalogRangeMismatch = errors.New("the download server returned a different byte range; the next attempt restarts from the beginning")
	errCatalogStalled       = errors.New("the download stalled without receiving data")
)

// StartCatalogDownload downloads one curated model into the managed store.
// Callers pass an entry from FindCatalogModel; HTTP handlers never accept a
// free-form URL. A model already in the store with the same digest completes
// immediately without touching the network.
func (m *ModelManager) StartCatalogDownload(ctx context.Context, model CatalogModel) (ImportJob, error) {
	if len(model.SHA256) != sha256.Size*2 || model.SizeBytes <= 0 || model.DownloadURL == "" {
		return ImportJob{}, fmt.Errorf("catalog model %q is not pinned", model.ID)
	}
	installedID, err := m.CatalogModelInstalled(ctx, model)
	if err != nil {
		return ImportJob{}, err
	}
	if installedID != "" {
		return m.recordCompletedImport(catalogSource(model), model.DisplayName, model.SizeBytes, installedID)
	}
	return m.startImport(modelImportSpec{
		DisplayName:   model.DisplayName,
		Source:        catalogSource(model),
		SourceName:    model.SourceName,
		SourcePath:    model.DownloadURL,
		ExpectedSHA:   model.SHA256,
		SizeBytes:     model.SizeBytes,
		Format:        "gguf",
		Family:        model.Family,
		ParameterSize: model.ParameterSize,
		Quantization:  model.Quantization,
		License:       model.License,
	}, m.runCatalogDownload)
}

// CatalogModelInstalled returns the ready managed model holding a curated digest.
func (m *ModelManager) CatalogModelInstalled(ctx context.Context, model CatalogModel) (string, error) {
	existing, ok, err := m.modelBySHA(ctx, model.SHA256)
	if err != nil {
		return "", modelInventoryError("find catalog model", err)
	}
	if !ok || existing.State != modelStateReady {
		return "", nil
	}
	return existing.ID, nil
}

// CatalogPartialBytes reports how much of a curated model a resumable partial holds.
func (m *ModelManager) CatalogPartialBytes(model CatalogModel) int64 {
	info, err := os.Stat(filepath.Join(m.downloadsDir, catalogPartialName(model.SHA256)))
	if err != nil || !info.Mode().IsRegular() || info.Size() > model.SizeBytes {
		return 0
	}
	return info.Size()
}

// DownloadsDir is where partial imports and catalog downloads are staged.
func (m *ModelManager) DownloadsDir() string {
	return m.downloadsDir
}

func (m *ModelManager) runCatalogDownload(ctx context.Context, jobID string, spec modelImportSpec) {
	defer m.wg.Done()
	partial := filepath.Join(m.downloadsDir, catalogPartialName(spec.ExpectedSHA))
	digest, err := m.downloadCatalogPartial(ctx, jobID, spec, partial)
	if err != nil {
		// Transport failures and cancellation keep the partial so the next
		// attempt resumes; checksum failures have already discarded it.
		m.finishImport(jobID, "", err)
		return
	}
	if _, err := validateGGUFFile(partial); err != nil {
		m.finishImport(jobID, "", cleanupImportPartial(partial, fmt.Errorf("validate downloaded model: %w", err)))
		return
	}
	record, err := m.commitModel(ctx, spec, partial, digest, spec.SizeBytes)
	if err != nil {
		m.finishImport(jobID, "", cleanupImportPartial(partial, err))
		return
	}
	m.finishImport(jobID, record.ID, nil)
}

func (m *ModelManager) downloadCatalogPartial(ctx context.Context, jobID string, spec modelImportSpec, partial string) (string, error) {
	client := m.downloadClient
	if client == nil {
		transport := &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			ResponseHeaderTimeout: 60 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
		}
		defer transport.CloseIdleConnections()
		client = &http.Client{Transport: transport}
	}

	var lastErr error
	for attempt := 1; attempt <= catalogDownloadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		digest, err := m.downloadCatalogAttempt(ctx, client, jobID, spec, partial)
		if err == nil {
			return digest, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, errCatalogChecksum) {
			return "", err
		}
		lastErr = err
		if attempt < catalogDownloadAttempts {
			timer := time.NewTimer(time.Duration(attempt) * m.downloadRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
	}
	return "", fmt.Errorf("model download failed after %d attempts: %w; the partial download was kept so a retry resumes", catalogDownloadAttempts, lastErr)
}

// downloadCatalogAttempt resumes the partial from its current size. The saved
// prefix is hashed first, so the digest covers every byte without a second pass.
func (m *ModelManager) downloadCatalogAttempt(
	ctx context.Context,
	client *http.Client,
	jobID string,
	spec modelImportSpec,
	partial string,
) (string, error) {
	digest := sha256.New()
	offset, err := hashPartialPrefix(partial, spec.SizeBytes, digest)
	if err != nil {
		return "", err
	}
	if offset == spec.SizeBytes {
		sum, err := finishCatalogDigest(digest, spec.ExpectedSHA, partial)
		if !errors.Is(err, errCatalogChecksum) {
			return sum, err
		}
		// A complete partial from an earlier run was corrupt and has been
		// discarded; download the file again instead of failing outright.
		offset = 0
		digest.Reset()
	}

	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, spec.SourcePath, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", catalogDownloadUserAgent)
	request.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()

	resume, err := catalogResponseResume(response, offset, spec.SizeBytes)
	if err != nil {
		if errors.Is(err, errCatalogRangeMismatch) {
			_ = os.Remove(partial)
		}
		return "", err
	}
	if !resume {
		offset = 0
		digest.Reset()
	}
	written, err := m.writeCatalogBody(attemptCtx, cancel, jobID, spec, partial, response.Body, offset, digest)
	if err != nil {
		return "", err
	}
	if written != spec.SizeBytes {
		return "", fmt.Errorf("download ended early at %d of %d bytes: %w", written, spec.SizeBytes, io.ErrUnexpectedEOF)
	}
	return finishCatalogDigest(digest, spec.ExpectedSHA, partial)
}

// catalogResponseResume reports whether the response continues the saved
// partial (true) or restarts the file from byte zero (false).
func catalogResponseResume(response *http.Response, offset, size int64) (bool, error) {
	switch response.StatusCode {
	case http.StatusOK:
		if response.ContentLength >= 0 && response.ContentLength != size {
			return false, fmt.Errorf("download server reports %d bytes; the catalog pins %d", response.ContentLength, size)
		}
		return false, nil
	case http.StatusPartialContent:
		start, total, ok := parseContentRange(response.Header.Get("Content-Range"))
		if !ok || start != offset {
			return false, errCatalogRangeMismatch
		}
		if total > 0 && total != size {
			return false, fmt.Errorf("download server reports %d bytes; the catalog pins %d", total, size)
		}
		return offset > 0, nil
	case http.StatusRequestedRangeNotSatisfiable:
		return false, errCatalogRangeMismatch
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return false, fmt.Errorf("download server returned HTTP %d", response.StatusCode)
	}
}

func (m *ModelManager) writeCatalogBody(
	ctx context.Context,
	cancel context.CancelFunc,
	jobID string,
	spec modelImportSpec,
	partial string,
	body io.Reader,
	offset int64,
	digest hash.Hash,
) (int64, error) {
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if offset > 0 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	// #nosec G304 -- partial is derived from a pinned digest under the private downloads directory.
	output, err := os.OpenFile(partial, flags, 0o600)
	if err != nil {
		return offset, fmt.Errorf("open model download: %w", err)
	}
	// A stalled connection sends neither bytes nor an error. Cancel the attempt
	// so the next one resumes instead of waiting forever.
	var stalled atomic.Bool
	idle := time.AfterFunc(m.downloadIdleLimit, func() {
		stalled.Store(true)
		cancel()
	})
	defer idle.Stop()

	completed := offset
	m.updateImportProgress(jobID, ImportStatusDownloading, completed)
	var lastProgress time.Time // the first chunk reports at once; later ones are throttled
	buffer := make([]byte, catalogDownloadBufferSize)
	var copyErr error
	for {
		count, readErr := body.Read(buffer)
		if count > 0 {
			idle.Reset(m.downloadIdleLimit)
			if completed+int64(count) > spec.SizeBytes {
				_ = output.Close()
				_ = os.Remove(partial)
				return completed, fmt.Errorf("download exceeded the pinned size of %d bytes", spec.SizeBytes)
			}
			if _, err := output.Write(buffer[:count]); err != nil {
				copyErr = err
				break
			}
			_, _ = digest.Write(buffer[:count])
			completed += int64(count)
			if time.Since(lastProgress) >= catalogProgressInterval {
				m.updateImportProgress(jobID, ImportStatusDownloading, completed)
				lastProgress = time.Now()
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			copyErr = readErr
			break
		}
		if err := ctx.Err(); err != nil {
			copyErr = err
			break
		}
	}
	if stalled.Load() {
		copyErr = errCatalogStalled
	}
	if syncErr := output.Sync(); copyErr == nil && syncErr != nil {
		copyErr = syncErr
	}
	if closeErr := output.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	m.updateImportProgress(jobID, ImportStatusDownloading, completed)
	return completed, copyErr
}

// hashPartialPrefix feeds an existing partial into digest and returns its size.
// An oversized partial cannot belong to the pinned file and is discarded.
func hashPartialPrefix(partial string, size int64, digest hash.Hash) (int64, error) {
	file, err := os.Open(partial) // #nosec G304 -- private downloads directory, pinned-digest file name.
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open saved partial download: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return 0, fmt.Errorf("inspect saved partial download: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > size {
		_ = file.Close()
		if removeErr := os.Remove(partial); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return 0, fmt.Errorf("discard oversized partial download: %w", removeErr)
		}
		return 0, nil
	}
	copied, err := io.Copy(digest, file)
	_ = file.Close()
	if err != nil {
		return 0, fmt.Errorf("read saved partial download: %w", err)
	}
	return copied, nil
}

func finishCatalogDigest(digest hash.Hash, expected, partial string) (string, error) {
	actual := hex.EncodeToString(digest.Sum(nil))
	if strings.EqualFold(actual, expected) {
		return actual, nil
	}
	if err := os.Remove(partial); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", errors.Join(errCatalogChecksum, err)
	}
	return "", errCatalogChecksum
}

// parseContentRange reads "bytes start-end/total"; total is 0 when unknown.
func parseContentRange(value string) (int64, int64, bool) {
	rangePart, ok := strings.CutPrefix(strings.TrimSpace(value), "bytes ")
	if !ok {
		return 0, 0, false
	}
	span, totalText, ok := strings.Cut(rangePart, "/")
	if !ok {
		return 0, 0, false
	}
	startText, _, ok := strings.Cut(span, "-")
	if !ok {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startText), 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if strings.TrimSpace(totalText) == "*" {
		return start, 0, true
	}
	total, err := strconv.ParseInt(strings.TrimSpace(totalText), 10, 64)
	if err != nil || total <= 0 {
		return 0, 0, false
	}
	return start, total, true
}

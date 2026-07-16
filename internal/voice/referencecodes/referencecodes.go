// Package referencecodes prepares NeuCodec reference tensors for the
// non-Python NeuTTS runner. It parses only the narrow tensor formats the runner
// consumes; Torch pickle payloads are inspected as data and are never executed.
package referencecodes

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	maxSourceBytes     = 8 << 20
	maxArchiveBytes    = 16 << 20
	maxPickleBytes     = 64 << 10
	maxReferenceTokens = 100_000
	maxReferenceWAV    = 32 << 20
	maxTranscriptBytes = 8 << 10
	referenceIDLength  = 24
)

var (
	descrPattern   = regexp.MustCompile(`['"]descr['"]\s*:\s*['"]([^'"]+)['"]`)
	fortranPattern = regexp.MustCompile(`['"]fortran_order['"]\s*:\s*(True|False)`)
	shapePattern   = regexp.MustCompile(`['"]shape['"]\s*:\s*\(\s*([0-9]+)\s*,\s*\)`)
	referenceID    = regexp.MustCompile(`^[0-9a-f]{24}$`)
)

// Request selects a pre-encoded tensor and, optionally, its reference audio.
// When ReferenceWAV is blank, an adjacent WAV with the source basename is used
// when present.
type Request struct {
	SourcePath   string
	ReferenceWAV string
}

// GenerateRequest contains the only user inputs needed to create a NeuCodec
// reference. The exact transcript conditions TTS but is not an encoder input.
type GenerateRequest struct {
	ReferenceWAV string
	Transcript   string
}

// Encoder identifies the app-managed native worker and its pinned ONNX model.
// Neither path is accepted from the browser.
type Encoder struct {
	Executable string
	Model      string
}

// Result describes the app-managed reference bundle.
type Result struct {
	ID           string `json:"id"`
	CodesPath    string `json:"codes_path"`
	AudioPath    string `json:"audio_path,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	TokenCount   int    `json:"token_count"`
	SourceFormat string `json:"source_format"`
	Reused       bool   `json:"reused"`
}

type managedBundle struct {
	codesPath string
	audioPath string
	reused    bool
}

type torchBundle struct {
	pickle    *zip.File
	data      *zip.File
	byteorder *zip.File
}

var encodeReferenceWAV = runEncoder

// Prepare validates and canonicalizes a supported reference-code source.
func Prepare(dataDir string, request Request) (Result, error) {
	root, err := managedReferenceRoot(dataDir)
	if err != nil {
		return Result{}, err
	}
	sourcePath, err := regularPath(request.SourcePath, maxSourceBytes)
	if err != nil {
		return Result{}, fmt.Errorf("reference-code source: %w", err)
	}
	codes, sourceFormat, err := readReferenceCodes(sourcePath)
	if err != nil {
		return Result{}, err
	}
	if err = validateCodes(codes); err != nil {
		return Result{}, err
	}
	canonical := encodeNPY(codes)
	wav, err := readReferenceWAV(sourcePath, request.ReferenceWAV)
	if err != nil {
		return Result{}, err
	}
	hash := sha256.New()
	_, _ = hash.Write(canonical)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(wav)
	id := fmt.Sprintf("%x", hash.Sum(nil))[:referenceIDLength]
	bundle, err := storeManagedBundle(root, id, canonical, wav)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ID:           id,
		CodesPath:    bundle.codesPath,
		AudioPath:    bundle.audioPath,
		Transcript:   adjacentTranscript(sourcePath),
		TokenCount:   len(codes),
		SourceFormat: sourceFormat,
		Reused:       bundle.reused,
	}, nil
}

// Generate encodes a local WAV with the installer-managed, non-Python worker,
// then parses and canonicalizes its output before publishing a managed bundle.
func Generate(ctx context.Context, dataDir string, request GenerateRequest, encoder Encoder) (Result, error) {
	transcript := strings.TrimSpace(request.Transcript)
	if transcript == "" {
		return Result{}, errors.New("the exact reference transcript is required")
	}
	if len(transcript) > maxTranscriptBytes {
		return Result{}, fmt.Errorf("reference transcript must not exceed %d KiB", maxTranscriptBytes>>10)
	}
	root, err := managedReferenceRoot(dataDir)
	if err != nil {
		return Result{}, err
	}
	wavPath, err := regularPath(request.ReferenceWAV, maxReferenceWAV)
	if err != nil {
		return Result{}, fmt.Errorf("reference audio: %w", err)
	}
	wav, err := os.ReadFile(wavPath) // #nosec G304 -- explicit controller-selected local WAV.
	if err != nil {
		return Result{}, fmt.Errorf("read reference audio: %w", err)
	}
	if !validWAV(wav) {
		return Result{}, errors.New("reference audio must be a valid RIFF/WAVE file")
	}
	temporary, err := os.MkdirTemp(root, ".encode-")
	if err != nil {
		return Result{}, fmt.Errorf("create reference encoding workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	encodedPath := filepath.Join(temporary, "reference.npy")
	if err = encodeReferenceWAV(ctx, encoder, wavPath, encodedPath); err != nil {
		return Result{}, err
	}
	codes, _, err := readReferenceCodes(encodedPath)
	if err != nil {
		return Result{}, fmt.Errorf("validate generated reference codes: %w", err)
	}
	if err = validateCodes(codes); err != nil {
		return Result{}, fmt.Errorf("validate generated reference codes: %w", err)
	}
	canonical := encodeNPY(codes)
	hash := sha256.New()
	_, _ = hash.Write(canonical)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(wav)
	id := fmt.Sprintf("%x", hash.Sum(nil))[:referenceIDLength]
	bundle, err := storeManagedBundle(root, id, canonical, wav)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ID:           id,
		CodesPath:    bundle.codesPath,
		AudioPath:    bundle.audioPath,
		Transcript:   transcript,
		TokenCount:   len(codes),
		SourceFormat: "neucodec_onnx",
		Reused:       bundle.reused,
	}, nil
}

func runEncoder(ctx context.Context, encoder Encoder, wavPath, outputPath string) error {
	executable, err := regularPath(encoder.Executable, 128<<20)
	if err != nil {
		return fmt.Errorf("NeuCodec reference encoder: %w", err)
	}
	model, err := regularPath(encoder.Model, 1<<30)
	if err != nil {
		return fmt.Errorf("NeuCodec encoder model: %w", err)
	}
	if _, err = regularPath(model+".data", 1<<30); err != nil {
		return fmt.Errorf("NeuCodec encoder model data: %w", err)
	}
	// #nosec G204 -- executable and model are fixed app-managed paths; the WAV
	// is an explicit local controller selection and no shell is involved.
	command := exec.CommandContext(ctx, executable,
		"--model", model,
		"--input", wavPath,
		"--output", outputPath,
	)
	command.Dir = filepath.Dir(executable)
	output := &outputTail{limit: 1024}
	command.Stdout = output
	command.Stderr = output
	runErr := command.Run()
	if runErr == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("NeuCodec reference encoding timed out")
	}
	message := "NeuCodec reference encoding failed"
	if detail := output.String(); detail != "" {
		message += ": " + detail
	}
	return errors.New(message)
}

func managedReferenceRoot(dataDir string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", errors.New("MagicHandy data directory is unavailable")
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", errors.New("MagicHandy data directory is invalid")
	}
	root := filepath.Join(absDataDir, "voice", "neutts", "references")
	if err = os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create reference directory: %w", err)
	}
	return root, nil
}

func readReferenceCodes(sourcePath string) ([]int32, string, error) {
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".pt", ".pth":
		codes, err := readTorchTensor(sourcePath)
		return codes, "torch_int32", err
	case ".npy":
		codes, err := readNPY(sourcePath)
		return codes, "npy_int32", err
	default:
		return nil, "", errors.New("supported sources are official NeuTTS-style .pt tensors and one-dimensional int32 .npy files")
	}
}

func readReferenceWAV(sourcePath, requestedPath string) ([]byte, error) {
	wavPath := strings.TrimSpace(requestedPath)
	if wavPath == "" {
		candidate := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".wav"
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			wavPath = candidate
		}
	}
	if wavPath == "" {
		return nil, nil
	}
	wavPath, err := regularPath(wavPath, maxReferenceWAV)
	if err != nil {
		return nil, fmt.Errorf("reference audio: %w", err)
	}
	wav, err := os.ReadFile(wavPath) // #nosec G304 -- explicit local path selected by the controller.
	if err != nil {
		return nil, fmt.Errorf("read reference audio: %w", err)
	}
	if !validWAV(wav) {
		return nil, errors.New("reference audio must be a valid RIFF/WAVE file")
	}
	return wav, nil
}

func storeManagedBundle(root, id string, canonical, wav []byte) (managedBundle, error) {
	codesPath := filepath.Join(root, id+".npy")
	reused, err := writeManagedFile(codesPath, canonical)
	if err != nil {
		return managedBundle{}, fmt.Errorf("store reference codes: %w", err)
	}
	bundle := managedBundle{codesPath: filepath.Clean(codesPath), reused: reused}
	if len(wav) == 0 {
		return bundle, nil
	}
	audioPath := filepath.Join(root, id+".wav")
	audioReused, err := writeManagedFile(audioPath, wav)
	if err != nil {
		return managedBundle{}, fmt.Errorf("store reference audio: %w", err)
	}
	bundle.audioPath = filepath.Clean(audioPath)
	bundle.reused = bundle.reused && audioReused
	return bundle, nil
}

// AudioPath resolves only app-managed reference audio IDs.
func AudioPath(dataDir, id string) (string, error) {
	if !referenceID.MatchString(id) {
		return "", errors.New("invalid NeuTTS reference ID")
	}
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", errors.New("MagicHandy data directory is unavailable")
	}
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", errors.New("MagicHandy data directory is invalid")
	}
	path := filepath.Join(dataDir, "voice", "neutts", "references", id+".wav")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return path, nil
}

func regularPath(path string, maxBytes int64) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("path is invalid")
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("file is unavailable")
	}
	if info.Size() <= 0 || info.Size() > maxBytes {
		return "", fmt.Errorf("file size must be between 1 byte and %d MiB", maxBytes>>20)
	}
	return filepath.Clean(absPath), nil
}

func readTorchTensor(path string) ([]int32, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.New("torch source is not a supported ZIP tensor archive")
	}
	defer func() { _ = reader.Close() }()
	selected, err := findTorchBundle(reader.File)
	if err != nil {
		return nil, err
	}
	pickle, err := readZipFile(selected.pickle, maxPickleBytes)
	if err != nil {
		return nil, fmt.Errorf("read Torch tensor metadata: %w", err)
	}
	order, err := readZipFile(selected.byteorder, 16)
	if err != nil || strings.TrimSpace(string(order)) != "little" {
		return nil, errors.New("torch tensor must use little-endian storage")
	}
	count, err := torchTensorCount(pickle)
	if err != nil {
		return nil, err
	}
	raw, err := readZipFile(selected.data, maxSourceBytes)
	if err != nil {
		return nil, fmt.Errorf("read Torch tensor data: %w", err)
	}
	if len(raw) != count*4 {
		return nil, errors.New("torch tensor shape does not match its int32 storage")
	}
	return decodeInt32(raw, binary.LittleEndian), nil
}

func findTorchBundle(files []*zip.File) (*torchBundle, error) {
	if len(files) == 0 || len(files) > 32 {
		return nil, errors.New("torch tensor archive has an unsupported file layout")
	}
	bundles := make(map[string]*torchBundle)
	var total uint64
	for _, file := range files {
		if file.UncompressedSize64 > maxArchiveBytes || total > maxArchiveBytes-file.UncompressedSize64 {
			return nil, errors.New("torch tensor archive expands beyond the safety limit")
		}
		total += file.UncompressedSize64
		if err := collectTorchEntry(bundles, file); err != nil {
			return nil, err
		}
	}
	if len(bundles) != 1 {
		return nil, errors.New("torch tensor archive must contain exactly one tensor bundle")
	}
	var selected *torchBundle
	for _, selected = range bundles {
		break
	}
	if selected == nil || selected.pickle == nil || selected.data == nil || selected.byteorder == nil {
		return nil, errors.New("torch tensor archive is missing data.pkl, data/0, or byteorder")
	}
	return selected, nil
}

func collectTorchEntry(bundles map[string]*torchBundle, file *zip.File) error {
	name := filepath.ToSlash(file.Name)
	if marker := strings.LastIndex(name, "/data/"); marker >= 0 && name[marker+len("/data/"):] != "0" {
		return errors.New("torch tensor archive contains unsupported additional storage")
	}
	var suffix string
	for _, candidate := range []string{"/data.pkl", "/data/0", "/byteorder"} {
		if strings.HasSuffix(name, candidate) {
			suffix = candidate
			break
		}
	}
	if suffix == "" {
		return nil
	}
	prefix := strings.TrimSuffix(name, suffix)
	item := bundles[prefix]
	if item == nil {
		item = &torchBundle{}
		bundles[prefix] = item
	}
	var target **zip.File
	switch suffix {
	case "/data.pkl":
		target = &item.pickle
	case "/data/0":
		target = &item.data
	default:
		target = &item.byteorder
	}
	if *target != nil {
		return errors.New("torch tensor archive contains duplicate entries")
	}
	*target = file
	return nil
}

func torchTensorCount(pickle []byte) (int, error) {
	rebuildMarker := []byte("torch._utils\n_rebuild_tensor_v2\n")
	storageMarker := []byte("torch\nIntStorage\n")
	if len(pickle) < 2 || pickle[0] != 0x80 || pickle[1] > 5 ||
		bytes.Count(pickle, rebuildMarker) != 1 || bytes.Count(pickle, storageMarker) != 1 {
		return 0, errors.New("torch source is not a supported one-dimensional int32 tensor")
	}
	storage := bytes.Index(pickle, storageMarker)
	relative := bytes.IndexByte(pickle[storage+len(storageMarker):], 'Q')
	if relative < 0 {
		return 0, errors.New("torch tensor metadata is missing persistent storage")
	}
	position := storage + len(storageMarker) + relative + 1
	offset, next, ok := pickleInt(pickle, position)
	if !ok || offset != 0 {
		return 0, errors.New("torch tensor storage offset is unsupported")
	}
	count, next, ok := pickleInt(pickle, next)
	if !ok || count < 1 || count > maxReferenceTokens || next >= len(pickle) || pickle[next] != 0x85 {
		return 0, errors.New("torch tensor must have one bounded dimension")
	}
	next = skipPickleMemo(pickle, next+1)
	stride, next, ok := pickleInt(pickle, next)
	if !ok || stride != 1 || next >= len(pickle) || pickle[next] != 0x85 {
		return 0, errors.New("torch tensor must be contiguous")
	}
	return count, nil
}

func pickleInt(data []byte, position int) (int, int, bool) {
	if position >= len(data) {
		return 0, position, false
	}
	switch data[position] {
	case 'K':
		if position+2 > len(data) {
			return 0, position, false
		}
		return int(data[position+1]), position + 2, true
	case 'M':
		if position+3 > len(data) {
			return 0, position, false
		}
		return int(binary.LittleEndian.Uint16(data[position+1:])), position + 3, true
	case 'J':
		if position+5 > len(data) {
			return 0, position, false
		}
		value := binary.LittleEndian.Uint32(data[position+1:])
		if value > maxReferenceTokens {
			return 0, position, false
		}
		return int(value), position + 5, true // #nosec G115 -- value is bounded to 100,000 above.
	default:
		return 0, position, false
	}
}

func skipPickleMemo(data []byte, position int) int {
	for position < len(data) {
		switch data[position] {
		case 'q':
			position += 2
		case 'r':
			position += 5
		case 0x94:
			position++
		default:
			return position
		}
	}
	return position
}

func readNPY(path string) ([]int32, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit local path selected by the controller.
	if err != nil {
		return nil, fmt.Errorf("read NumPy source: %w", err)
	}
	if len(data) < 10 || !bytes.Equal(data[:6], []byte("\x93NUMPY")) {
		return nil, errors.New("reference codes are not a NumPy array")
	}
	major := data[6]
	headerStart := 10
	headerLength := 0
	switch major {
	case 1:
		headerLength = int(binary.LittleEndian.Uint16(data[8:10]))
	case 2, 3:
		if len(data) < 12 {
			return nil, errors.New("NumPy header is truncated")
		}
		headerStart = 12
		headerLength = int(binary.LittleEndian.Uint32(data[8:12]))
	default:
		return nil, fmt.Errorf("NumPy version %d is unsupported", major)
	}
	if headerLength <= 0 || headerLength > maxPickleBytes || headerStart+headerLength > len(data) {
		return nil, errors.New("NumPy header is invalid")
	}
	header := string(data[headerStart : headerStart+headerLength])
	descr := descrPattern.FindStringSubmatch(header)
	fortran := fortranPattern.FindStringSubmatch(header)
	shape := shapePattern.FindStringSubmatch(header)
	if len(descr) != 2 || len(fortran) != 2 || len(shape) != 2 || fortran[1] != "False" {
		return nil, errors.New("NumPy source must be a one-dimensional C-order int32 array")
	}
	count, err := strconv.Atoi(shape[1])
	if err != nil || count < 1 || count > maxReferenceTokens {
		return nil, errors.New("NumPy reference-code count is outside the supported range")
	}
	var order binary.ByteOrder
	switch descr[1] {
	case "<i4":
		order = binary.LittleEndian
	case ">i4":
		order = binary.BigEndian
	default:
		return nil, errors.New("NumPy reference codes must use signed int32 values")
	}
	raw := data[headerStart+headerLength:]
	if len(raw) != count*4 {
		return nil, errors.New("NumPy shape does not match its int32 data")
	}
	return decodeInt32(raw, order), nil
}

func encodeNPY(codes []int32) []byte {
	header := fmt.Sprintf("{'descr': '<i4', 'fortran_order': False, 'shape': (%d,), }", len(codes))
	padding := 16 - ((10 + len(header) + 1) % 16)
	header += strings.Repeat(" ", padding) + "\n"
	data := make([]byte, 10+len(header)+len(codes)*4)
	copy(data, []byte("\x93NUMPY"))
	data[6], data[7] = 1, 0
	binary.LittleEndian.PutUint16(data[8:10], uint16(len(header))) // #nosec G115 -- bounded token count keeps this header below 64 KiB.
	copy(data[10:], header)
	position := 10 + len(header)
	for _, code := range codes {
		binary.LittleEndian.PutUint32(data[position:], uint32(code)) // #nosec G115 -- validateCodes limits values to 0..65535.
		position += 4
	}
	return data
}

func decodeInt32(raw []byte, order binary.ByteOrder) []int32 {
	values := make([]int32, len(raw)/4)
	for index := range values {
		values[index] = signedInt32(order.Uint32(raw[index*4:]))
	}
	return values
}

func signedInt32(value uint32) int32 {
	const maxInt32 = uint32(1<<31 - 1)
	if value <= maxInt32 {
		return int32(value) // #nosec G115 -- branch proves the conversion is in range.
	}
	return -int32(^value) - 1 // #nosec G115 -- complement is at most MaxInt32 in this branch.
}

func validateCodes(codes []int32) error {
	if len(codes) < 1 || len(codes) > maxReferenceTokens {
		return errors.New("reference-code count is outside the supported range")
	}
	for _, code := range codes {
		if code < 0 || code > 65_535 {
			return errors.New("reference codes contain a value outside the supported NeuCodec range")
		}
	}
	return nil
}

func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	if limit < 0 || file.UncompressedSize64 > uint64(limit) { // #nosec G115 -- negative limits are rejected before the conversion can be used.
		return nil, errors.New("archive entry exceeds the safety limit")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("archive entry exceeds the safety limit")
	}
	return data, nil
}

func validWAV(data []byte) bool {
	if len(data) < 44 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		return false
	}
	declaredSize := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declaredSize < 44 || declaredSize != uint64(len(data)) {
		return false
	}
	var foundFormat, foundData bool
	for offset := uint64(12); offset+8 <= declaredSize; {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkEnd < chunkStart || chunkEnd > declaredSize {
			return false
		}
		switch string(data[offset : offset+4]) {
		case "fmt ":
			foundFormat = chunkSize >= 16
		case "data":
			foundData = true
		}
		offset = chunkEnd + chunkSize%2
	}
	return foundFormat && foundData
}

func adjacentTranscript(sourcePath string) string {
	path := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".txt"
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxTranscriptBytes {
		return ""
	}
	data, err := os.ReadFile(path) // #nosec G304 -- adjacent to the explicit controller-selected source.
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeManagedFile(path string, data []byte) (bool, error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return false, errors.New("managed reference path is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if existing, err := os.ReadFile(path); err == nil { // #nosec G304 -- path is a SHA-addressed child of the app-managed reference root.
		if bytes.Equal(existing, data) {
			return true, nil
		}
		return false, errors.New("managed reference ID collision")
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".reference-*")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) { // #nosec G304 -- path is a SHA-addressed child of the app-managed reference root.
			return true, nil
		}
		return false, err
	}
	return false, nil
}

type outputTail struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *outputTail) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(data)
	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return written, nil
}

func (b *outputTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}

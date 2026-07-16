package referencecodes

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const officialTensorPickleHex = "800263746f7263682e5f7574696c730a5f72656275696c645f74656e736f725f76320a71002828580700000073746f72616765710163746f7263680a496e7453746f726167650a71025801000000307103580300000063707571044d7401747105514b004d74018571064b018571078963636f6c6c656374696f6e730a4f726465726564446963740a71082952710974710a52710b2e"

func TestGenerateReferenceFromWAV(t *testing.T) {
	directory := t.TempDir()
	wavPath := filepath.Join(directory, "speaker.wav")
	wav := fixtureWAV()
	if err := os.WriteFile(wavPath, wav, 0o600); err != nil {
		t.Fatal(err)
	}
	previous := encodeReferenceWAV
	encodeReferenceWAV = func(_ context.Context, _ Encoder, inputPath, outputPath string) error {
		if inputPath != wavPath {
			t.Fatalf("encoder input = %q, want %q", inputPath, wavPath)
		}
		return os.WriteFile(outputPath, encodeNPY([]int32{10, 20, 30}), 0o600)
	}
	defer func() { encodeReferenceWAV = previous }()

	result, err := Generate(context.Background(), filepath.Join(directory, "data"), GenerateRequest{
		ReferenceWAV: wavPath,
		Transcript:   "  The exact words spoken.  ",
	}, Encoder{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.SourceFormat != "neucodec_onnx" || result.TokenCount != 3 || result.Transcript != "The exact words spoken." {
		t.Fatalf("result = %+v", result)
	}
	if result.CodesPath == "" || result.AudioPath == "" {
		t.Fatalf("managed paths missing: %+v", result)
	}
	gotCodes, err := readNPY(result.CodesPath)
	if err != nil || !reflect.DeepEqual(gotCodes, []int32{10, 20, 30}) {
		t.Fatalf("generated codes = %v, %v", gotCodes, err)
	}
	gotWAV, err := os.ReadFile(result.AudioPath) // #nosec G304 -- app-managed test path.
	if err != nil || !reflect.DeepEqual(gotWAV, wav) {
		t.Fatalf("generated WAV = %d bytes, %v", len(gotWAV), err)
	}
}

func TestGenerateRequiresTranscriptBeforeRunningEncoder(t *testing.T) {
	directory := t.TempDir()
	wavPath := filepath.Join(directory, "speaker.wav")
	if err := os.WriteFile(wavPath, fixtureWAV(), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	previous := encodeReferenceWAV
	encodeReferenceWAV = func(context.Context, Encoder, string, string) error {
		called = true
		return nil
	}
	defer func() { encodeReferenceWAV = previous }()

	if _, err := Generate(context.Background(), filepath.Join(directory, "data"), GenerateRequest{ReferenceWAV: wavPath}, Encoder{}); err == nil {
		t.Fatal("blank transcript was accepted")
	}
	if called {
		t.Fatal("encoder ran before transcript validation")
	}
}

func TestPrepareOfficialTorchTensorWithPreview(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "dave.pt")
	want := make([]int32, 372)
	for index := range want {
		want[index] = int32(index % 4096)
	}
	writeTorchFixture(t, source, want)
	wav := fixtureWAV()
	if err := os.WriteFile(filepath.Join(directory, "dave.wav"), wav, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "dave.txt"), []byte("The exact reference sentence."), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Prepare(filepath.Join(directory, "data"), Request{SourcePath: source})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if result.TokenCount != len(want) || result.SourceFormat != "torch_int32" || result.Transcript != "The exact reference sentence." {
		t.Fatalf("result = %+v", result)
	}
	got, err := readNPY(result.CodesPath)
	if err != nil {
		t.Fatalf("read prepared NumPy: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("prepared codes differ from Torch storage")
	}
	previewPath, err := AudioPath(filepath.Join(directory, "data"), result.ID)
	if err != nil || previewPath != result.AudioPath {
		t.Fatalf("preview path = %q, %v; want %q", previewPath, err, result.AudioPath)
	}
	if gotWAV, err := os.ReadFile(previewPath); err != nil || !reflect.DeepEqual(gotWAV, wav) { // #nosec G304 -- previewPath is returned by AudioPath for a test temp directory.
		t.Fatalf("prepared WAV differs: %v", err)
	}

	reused, err := Prepare(filepath.Join(directory, "data"), Request{SourcePath: source})
	if err != nil || !reused.Reused || reused.ID != result.ID {
		t.Fatalf("repeat prepare = %+v, %v", reused, err)
	}
}

func TestOfficialSampleCompatibility(t *testing.T) {
	source := os.Getenv("MAGICHANDY_NEUTTS_SAMPLE")
	if source == "" {
		t.Skip("set MAGICHANDY_NEUTTS_SAMPLE to an official NeuTTS .pt sample")
	}
	result, err := Prepare(t.TempDir(), Request{SourcePath: source})
	if err != nil {
		t.Fatalf("prepare official sample: %v", err)
	}
	if result.SourceFormat != "torch_int32" || result.TokenCount < 1 {
		t.Fatalf("official sample result = %+v", result)
	}
	if _, err = readNPY(result.CodesPath); err != nil {
		t.Fatalf("prepared official sample is not runner-compatible NPY: %v", err)
	}
}

func TestPrepareCanonicalizesBigEndianNPY(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "voice.npy")
	header := "{'descr': '>i4', 'fortran_order': False, 'shape': (3,), }          \n"
	data := make([]byte, 10+len(header)+12)
	copy(data, []byte("\x93NUMPY"))
	data[6] = 1
	binary.LittleEndian.PutUint16(data[8:10], uint16(len(header))) // #nosec G115 -- fixed test header is below 64 KiB.
	copy(data[10:], header)
	for index, value := range []uint32{4, 8, 15} {
		binary.BigEndian.PutUint32(data[10+len(header)+index*4:], value)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Prepare(filepath.Join(directory, "data"), Request{SourcePath: source})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	got, err := readNPY(result.CodesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int32{4, 8, 15}) {
		t.Fatalf("codes = %v", got)
	}
	if result.AudioPath != "" {
		t.Fatalf("missing preview audio became %q", result.AudioPath)
	}
}

func TestPrepareRejectsUnsupportedTorchPayload(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "unsafe.pt")
	file, err := os.Create(source) // #nosec G304 -- source is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("unsafe/data.pkl")
	_, _ = entry.Write([]byte("cos\nsystem\n"))
	entry, _ = archive.Create("unsafe/data/0")
	_, _ = entry.Write(make([]byte, 4))
	entry, _ = archive.Create("unsafe/byteorder")
	_, _ = entry.Write([]byte("little"))
	_ = archive.Close()
	_ = file.Close()

	if _, err = Prepare(filepath.Join(directory, "data"), Request{SourcePath: source}); err == nil {
		t.Fatal("unsupported pickle payload was accepted")
	}
	if _, err = AudioPath(filepath.Join(directory, "data"), "../../private"); err == nil {
		t.Fatal("path-like reference ID was accepted")
	}
	if _, err = AudioPath("", "abcdef0123456789abcdef01"); err == nil {
		t.Fatal("blank data directory was accepted")
	}
}

func TestPrepareRejectsAdditionalTorchStorage(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "multiple.pt")
	values := []int32{1, 2, 3}
	// Rebuild the fixture with an extra tensor storage entry; the parser must
	// reject the archive rather than guessing which tensor the pickle intended.
	pickle, _ := hex.DecodeString(officialTensorPickleHex)
	file, err := os.Create(source) // #nosec G304 -- source is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, data := range map[string][]byte{
		"voice/data.pkl":  pickle,
		"voice/byteorder": []byte("little"),
		"voice/data/0":    make([]byte, len(values)*4),
		"voice/data/1":    make([]byte, 4),
	} {
		entry, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write(data); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Prepare(filepath.Join(directory, "data"), Request{SourcePath: source}); err == nil {
		t.Fatal("Torch archive with additional storage was accepted")
	}
}

func TestValidWAVRequiresRealChunks(t *testing.T) {
	data := fixtureWAV()
	copy(data[12:16], "JUNK")
	copy(data[20:24], "fmt ")
	copy(data[28:32], "data")
	if validWAV(data) {
		t.Fatal("WAV accepted marker text inside a non-format chunk")
	}
}

func TestValidWAVRejectsDataOutsideRIFFContainer(t *testing.T) {
	data := append(fixtureWAV(), []byte("hidden trailing data")...)
	if validWAV(data) {
		t.Fatal("WAV accepted bytes outside its declared RIFF container")
	}
}

func writeTorchFixture(t *testing.T, path string, values []int32) {
	t.Helper()
	pickle, err := hex.DecodeString(officialTensorPickleHex)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path) // #nosec G304 -- callers provide paths inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, data := range map[string][]byte{
		"dave/data.pkl":  pickle,
		"dave/byteorder": []byte("little"),
		"dave/version":   []byte("3\n"),
	} {
		entry, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write(data); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	entry, err := archive.Create("dave/data/0")
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(raw[index*4:], uint32(value)) // #nosec G115 -- fixture values are nonnegative NeuCodec codes.
	}
	if _, err = entry.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fixtureWAV() []byte {
	data := make([]byte, 44)
	copy(data[:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8)) // #nosec G115 -- fixed 44-byte fixture.
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], 24_000)
	binary.LittleEndian.PutUint32(data[28:32], 48_000)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	return data
}

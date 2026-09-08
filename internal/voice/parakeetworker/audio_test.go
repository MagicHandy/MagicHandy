package parakeetworker

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func TestHealthClearsReadyAfterManagedChildExit(t *testing.T) {
	var output bytes.Buffer
	s := &session{loaded: true, runner: &managedServer{}, writer: &output}
	s.handleHealth(protocol.Request{ID: "health"})
	var response protocol.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if s.loaded || response.Type != protocol.ResponseError {
		t.Fatalf("stale readiness: %+v", response)
	}
}

type countingAudioReader struct {
	io.Reader
	bytesRead int
}

func (r *countingAudioReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += n
	return n, err
}

func TestMultipartDoesNotReadAudioUntilTransportConsumesIt(t *testing.T) {
	input := &countingAudioReader{Reader: strings.NewReader("audio-data")}
	body, contentType, length, err := multipartAudio(input, 10, "wav", "parakeet")
	if err != nil {
		t.Fatal(err)
	}
	if input.bytesRead != 0 {
		t.Fatal("multipart construction eagerly copied audio")
	}
	encoded, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(encoded)) != length {
		t.Fatalf("content-length %d != %d", length, len(encoded))
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(bytes.NewReader(encoded), params["boundary"]).ReadForm(1024)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = form.RemoveAll() }()
	if form.Value["model"][0] != "parakeet" || form.File["file"][0].Size != 10 {
		t.Fatalf("invalid multipart: %+v", form)
	}
}

func BenchmarkMultipartAudio32MiB(b *testing.B) {
	audio := make([]byte, 32<<20)
	b.SetBytes(int64(len(audio)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		body, _, _, err := multipartAudio(bytes.NewReader(audio), int64(len(audio)), "wav", "parakeet")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			b.Fatal(err)
		}
	}
}

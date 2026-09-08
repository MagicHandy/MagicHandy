package voice

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestAudioChunksAreAvailableBeforeCompletionAndStopRevokesThem(t *testing.T) {
	p := &PendingRequest{Type: RequestSpeak, Role: RoleTTS, state: RequestStateActive}
	if err := p.appendAudio(Response{AudioFormat: "pcm_s16le_24000", AudioB64: "AQI="}); err != nil {
		t.Fatal(err)
	}
	chunk, err := p.AudioChunk(0)
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Done || chunk.NextOffset != 2 || len(chunk.Data) != 2 {
		t.Fatalf("chunk: %+v", chunk)
	}
	p.invalidate()
	chunk, err = p.AudioChunk(0)
	if err != nil {
		t.Fatal(err)
	}
	if chunk.State != RequestStateCanceled || len(chunk.Data) != 0 {
		t.Fatalf("Stop exposed stale audio: %+v", chunk)
	}
	if audio, _ := p.Audio(); len(audio) != 0 {
		t.Fatal("clip endpoint exposed invalidated audio")
	}
}

func TestCompletedWAVRepairsStreamingLengthsAtPlaybackBoundary(t *testing.T) {
	data := make([]byte, 48)
	copy(data, "RIFF")
	copy(data[8:], "WAVE")
	copy(data[12:], "fmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[4:], ^uint32(0))
	binary.LittleEndian.PutUint32(data[40:], ^uint32(0))
	p := &PendingRequest{Type: RequestSpeak, Role: RoleTTS, state: RequestStateActive}
	if err := p.appendAudio(Response{AudioFormat: "wav", AudioB64: base64.StdEncoding.EncodeToString(data)}); err != nil {
		t.Fatal(err)
	}
	if err := p.completeAudio(); err != nil {
		t.Fatal(err)
	}
	audio, _ := p.Audio()
	if binary.LittleEndian.Uint32(audio[4:]) != 40 || binary.LittleEndian.Uint32(audio[40:]) != 4 {
		t.Fatal("completed WAV retained unknown lengths")
	}
}

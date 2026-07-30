package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// cardPNG builds a card-bearing PNG the way Tavern-family tools do: base64
// JSON in a tEXt chunk keyed chara, inserted before IEND.
func cardPNG(t *testing.T, payload []byte) []byte {
	t.Helper()
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	data := img.Bytes()
	body := append(append(append([]byte("tEXt"), []byte("chara")...), 0),
		[]byte(base64.StdEncoding.EncodeToString(payload))...)
	var out bytes.Buffer
	out.Write(data[:len(data)-12])
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(body)-4)) // #nosec G115 -- fixture chunk is tiny
	out.Write(length[:])
	out.Write(body)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(body))
	out.Write(crc[:])
	out.Write(data[len(data)-12:])
	return out.Bytes()
}

func testCardJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"spec": "chara_card_v2",
		"data": map[string]any{
			"name":        "Annabelle",
			"description": "A shy step-sister.",
			"personality": "Curious.",
			"scenario":    "Late night.",
			"first_mes":   "*{{char}} looks up.* Oh, it's {{user}}.",
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return payload
}

func importCardBody(t *testing.T, server *Server, body []byte) (*httptest.ResponseRecorder, personasResponse) {
	t.Helper()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/personas/import", bytes.NewReader(body)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	var decoded personasResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder, decoded
}

func TestPersonaImportAcceptsCardPNG(t *testing.T) {
	server := newTestServer(t)
	recorder, decoded := importCardBody(t, server, cardPNG(t, testCardJSON(t)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if decoded.Persona == nil || decoded.Persona.Name != "Annabelle" {
		t.Fatalf("persona = %+v", decoded.Persona)
	}
	if decoded.Persona.Greeting == "" {
		t.Fatal("expected greeting from first_mes")
	}
}

func TestPersonaImportAcceptsCardJSON(t *testing.T) {
	server := newTestServer(t)
	recorder, decoded := importCardBody(t, server, testCardJSON(t))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if decoded.Persona == nil || decoded.Persona.Name != "Annabelle" {
		t.Fatalf("persona = %+v", decoded.Persona)
	}
}

func TestPersonaImportRejectsPNGWithoutCard(t *testing.T) {
	server := newTestServer(t)
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	recorder, _ := importCardBody(t, server, img.Bytes())
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPersonaImportFromURL(t *testing.T) {
	server := newTestServer(t)
	card := cardPNG(t, testCardJSON(t))
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(card)
	}))
	defer remote.Close()

	body, err := json.Marshal(map[string]string{"url": remote.URL + "/card.png"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := withController(httptest.NewRequest(http.MethodPost, "/api/personas/import-url", bytes.NewReader(body)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var decoded personasResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	if decoded.Persona == nil || decoded.Persona.Name != "Annabelle" {
		t.Fatalf("persona = %+v", decoded.Persona)
	}
}

func TestPersonaImportFromURLWithoutCardDataFails(t *testing.T) {
	server := newTestServer(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>Log in to view</body></html>"))
	}))
	defer remote.Close()

	body, err := json.Marshal(map[string]string{"url": remote.URL + "/character/1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := withController(httptest.NewRequest(http.MethodPost, "/api/personas/import-url", bytes.NewReader(body)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAttachingPersonaSeedsGreetingIntoEmptySession(t *testing.T) {
	server := newTestServer(t)
	_, decoded := importCardBody(t, server, cardPNG(t, testCardJSON(t)))
	if decoded.Persona == nil {
		t.Fatal("import failed")
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	recorder, _ := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+sessionID+"/persona",
		map[string]string{"persona_id": decoded.Persona.ID})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	messages, err := server.chatLog.RecentSession(sessionID, 5)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want the seeded greeting", len(messages))
	}
	if messages[0].Role != chat.MessageRoleAssistant {
		t.Fatalf("role = %q", messages[0].Role)
	}
	if messages[0].Content != "*Annabelle looks up.* Oh, it's you." {
		t.Fatalf("content = %q", messages[0].Content)
	}
	if messages[0].Diagnostics == nil || messages[0].Diagnostics.PersonaName != "Annabelle" {
		t.Fatalf("diagnostics = %+v, want the persona name for chat display", messages[0].Diagnostics)
	}
}

func TestAttachingPersonaDoesNotSeedIntoNonEmptySession(t *testing.T) {
	server := newTestServer(t)
	_, decoded := importCardBody(t, server, cardPNG(t, testCardJSON(t)))
	if decoded.Persona == nil {
		t.Fatal("import failed")
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleUser, "hello", "", nil); err != nil {
		t.Fatalf("append: %v", err)
	}
	recorder, _ := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+sessionID+"/persona",
		map[string]string{"persona_id": decoded.Persona.ID})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	messages, err := server.chatLog.RecentSession(sessionID, 5)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want only the user message", len(messages))
	}
}

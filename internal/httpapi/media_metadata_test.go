package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/media"
)

func scanMetadataFixture(t *testing.T, server *Server, names ...string) map[string]string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name+".mp4"), []byte("video "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := server.media.StartScanWithOptions([]string{root}, media.DefaultScanOptions()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	waitForMediaScan(t, server)
	videos, err := server.media.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]string, len(videos))
	for _, video := range videos {
		ids[video.DisplayName] = video.ID
	}
	return ids
}

func serveMetadata(t *testing.T, server *Server, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

// Curation is library content, not a device command: it needs no controller
// lease, so a phone can tag a video while the desktop keeps control.
func TestMediaMetadataRoundTripWithoutControllerLease(t *testing.T) {
	server := newTestServer(t)
	ids := scanMetadataFixture(t, server, "alpha", "beta")

	patch := serveMetadata(t, server, http.MethodPatch, "/api/media/videos/"+ids["alpha"]+"/metadata",
		`{"title":"  Evening take ","rating":4,"notes":"Good first half.","tags":["calm","Long","calm"]}`, nil)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", patch.Code, patch.Body.String())
	}
	var patched struct {
		Video media.Video `json:"video"`
	}
	if err := json.Unmarshal(patch.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Video.Title == nil || *patched.Video.Title != "Evening take" || len(patched.Video.Tags) != 2 {
		t.Fatalf("patched video = %+v", patched.Video)
	}

	bulk := serveMetadata(t, server, http.MethodPost, "/api/media/videos/tags",
		`{"ids":["`+ids["alpha"]+`","`+ids["beta"]+`"],"add":["Build"],"remove":["long"]}`, nil)
	if bulk.Code != http.StatusOK {
		t.Fatalf("bulk status = %d: %s", bulk.Code, bulk.Body.String())
	}

	tags := serveMetadata(t, server, http.MethodGet, "/api/media/tags", "", nil)
	var listed struct {
		Tags []media.TagCount `json:"tags"`
	}
	if err := json.Unmarshal(tags.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tags) != 2 || listed.Tags[0] != (media.TagCount{Tag: "Build", Count: 2}) || listed.Tags[1] != (media.TagCount{Tag: "calm", Count: 1}) {
		t.Fatalf("tags = %+v", listed.Tags)
	}

	rename := serveMetadata(t, server, http.MethodPost, "/api/media/tags/rename", `{"from":"calm","to":"Slow"}`, nil)
	if rename.Code != http.StatusOK || !strings.Contains(rename.Body.String(), `"renamed":1`) || !strings.Contains(rename.Body.String(), `"Slow"`) {
		t.Fatalf("rename status = %d: %s", rename.Code, rename.Body.String())
	}
	remove := serveMetadata(t, server, http.MethodPost, "/api/media/tags/delete", `{"tag":"build"}`, nil)
	if remove.Code != http.StatusOK || !strings.Contains(remove.Body.String(), `"removed":2`) {
		t.Fatalf("delete status = %d: %s", remove.Code, remove.Body.String())
	}

	list := serveMetadata(t, server, http.MethodGet, "/api/media/videos", "", nil)
	if !strings.Contains(list.Body.String(), `"title":"Evening take"`) || !strings.Contains(list.Body.String(), `"tags":["Slow"]`) ||
		!strings.Contains(list.Body.String(), `"tags":[]`) {
		t.Fatalf("catalog does not carry curation: %s", list.Body.String())
	}
}

func TestMediaMetadataRejectsInvalidAndUnknownInput(t *testing.T) {
	server := newTestServer(t)
	ids := scanMetadataFixture(t, server, "alpha")
	for _, testCase := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPatch, "/api/media/videos/" + ids["alpha"] + "/metadata", `{}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/media/videos/" + ids["alpha"] + "/metadata", `{"rating":7}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/media/videos/" + ids["alpha"] + "/metadata", `{"tags":["a,b"]}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/media/videos/" + ids["alpha"] + "/metadata", `{"unknown":true}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/media/videos/missing/metadata", `{"rating":3}`, http.StatusNotFound},
		{http.MethodPost, "/api/media/videos/tags", `{"ids":["missing"],"add":["x"]}`, http.StatusNotFound},
		{http.MethodPost, "/api/media/videos/tags", `{"ids":[],"add":["x"]}`, http.StatusBadRequest},
		{http.MethodPost, "/api/media/tags/rename", `{"from":"","to":"x"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/media/tags/delete", `{"tag":"  "}`, http.StatusBadRequest},
	} {
		recorder := serveMetadata(t, server, testCase.method, testCase.path, testCase.body, nil)
		if recorder.Code != testCase.want {
			t.Errorf("%s %s %s status = %d, want %d: %s", testCase.method, testCase.path, testCase.body, recorder.Code, testCase.want, recorder.Body.String())
		}
	}
}

// Titles are already shared with observers, so curation joins them; host
// roots stay administrator-only.
func TestObserversSeeCurationButNotHostRoots(t *testing.T) {
	server, store, admin, _ := newControllerSessionFixture(t)
	ids := scanMetadataFixture(t, server, "alpha")
	tags := []string{"calm"}
	if _, err := server.media.UpdateMetadata(t.Context(), ids["alpha"], media.MetadataPatch{Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	observer := newAdmissionIdentity(t, store, admin.ID, "observer", false)

	list := serveMetadata(t, server, http.MethodGet, "/api/media/videos", "", observer.cookie)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"tags":["calm"]`) {
		t.Fatalf("observer catalog = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), filepath.ToSlash(os.TempDir())) || strings.Contains(list.Body.String(), `"location_path":"`+"C:") {
		t.Fatalf("observer catalog exposed a host root: %s", list.Body.String())
	}
	write := serveMetadata(t, server, http.MethodPatch, "/api/media/videos/"+ids["alpha"]+"/metadata", `{"rating":2}`, observer.cookie)
	if write.Code != http.StatusForbidden {
		t.Fatalf("observer metadata write status = %d: %s", write.Code, write.Body.String())
	}
}

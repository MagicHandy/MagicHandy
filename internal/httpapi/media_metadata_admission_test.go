package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type metadataAdmissionReader struct {
	io.Reader
	before func()
}

func (r *metadataAdmissionReader) Read(p []byte) (int, error) {
	if r.before != nil {
		callback := r.before
		r.before = nil
		callback()
	}
	return r.Reader.Read(p)
}

func TestMediaMetadataRechecksPermissionAfterReadingTheBody(t *testing.T) {
	for _, revoke := range []string{"logout", "control"} {
		t.Run(revoke, func(t *testing.T) {
			server, store, admin, _ := newControllerSessionFixture(t)
			ids := scanMetadataFixture(t, server, "clip")
			operator := newAdmissionIdentity(t, store, admin.ID, "metadata-operator", true)
			current, err := store.InspectSession(t.Context(), operator.cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPatch, "/api/media/videos/"+ids["clip"]+"/metadata", &metadataAdmissionReader{
				Reader: strings.NewReader(`{"title":"must not be saved"}`),
				before: func() {
					var err error
					if revoke == "logout" {
						err = store.RevokeSession(t.Context(), operator.cookie.Value)
					} else {
						err = store.RevokeControl(t.Context(), admin.ID, current.Account.ID)
					}
					if err != nil {
						t.Fatal(err)
					}
				},
			})
			request.AddCookie(operator.cookie)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
				t.Fatalf("revoked metadata write: %d %s", response.Code, response.Body.String())
			}
			video, err := server.media.Video(t.Context(), ids["clip"])
			if err != nil {
				t.Fatal(err)
			}
			if video.Title != nil {
				t.Fatal("revoked request changed the video title")
			}
		})
	}
}

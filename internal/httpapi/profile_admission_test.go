package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProfileUploadCannotCommitAfterLogout(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	r := httptest.NewRequest(http.MethodPut, "/api/auth/profile-image", &metadataAdmissionReader{
		Reader: bytes.NewReader(profileImageFixture(t)),
		before: func() {
			if err := store.RevokeSession(t.Context(), cookie.Value); err != nil {
				t.Fatal(err)
			}
		},
	})
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked upload: %d %s", w.Code, w.Body.String())
	}
	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range listed {
		if account.ID == admin.ID && account.HasProfileImage {
			t.Fatal("revoked profile was written")
		}
	}
}

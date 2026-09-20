package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/notices"
)

func (s *Server) noticePreferenceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notice-preferences", s.handleNoticePreferences)
	mux.HandleFunc("PUT /api/notice-preferences", s.handleNoticePreferences)
	mux.HandleFunc("DELETE /api/notice-preferences", s.handleNoticePreferences)
}

func (s *Server) noticeBrowser(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := "MagicHandy-Notices"
	if s.auth.options.SecureCookies {
		name = "__Host-MagicHandy-Notices"
	}
	cookie, err := r.Cookie(name)
	var token string
	if err == nil {
		if decoded, decodeErr := base64.RawURLEncoding.DecodeString(cookie.Value); decodeErr == nil && len(decoded) == 32 && len(cookie.Value) == 43 {
			token = cookie.Value
		}
	}
	if token == "" {
		if !readRequest(r) {
			rejectRequest(w, r, http.StatusBadRequest, errors.New("load notice preferences before saving"))
			return "", false
		}
		var random [32]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("notice preferences are temporarily unavailable"))
			return "", false
		}
		token = base64.RawURLEncoding.EncodeToString(random[:])
	}
	// This identifier carries no login/control authority and is never returned
	// in JSON or stored in plaintext. Other browsers cannot select its record.
	// #nosec G124 -- the non-Secure variant is limited to trusted loopback HTTP,
	// matching the existing session-cookie policy; HTTPS uses __Host- and Secure.
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/", HttpOnly: true, Secure: s.auth.options.SecureCookies,
		SameSite: http.SameSiteStrictMode, MaxAge: int(notices.BrowserLifetime.Seconds())})
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:]), true
}

func (s *Server) handleNoticePreferences(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	w.Header().Set("Cache-Control", "no-store")
	browserHash, ok := s.noticeBrowser(w, r)
	if !ok {
		return
	}
	owner := notices.Owner{BrowserHash: browserHash}
	session, authenticated := authenticatedSession(r)
	if authenticated {
		owner.AccountID = session.session.Account.ID
	}
	if !s.requireNoticeScope(w, r, authenticated) {
		return
	}
	body, ok := decodeNoticePreference(w, r)
	if !ok {
		return
	}
	db := s.store.Datastore()
	if !readRequest(r) {
		err := db.WithTx(ctx, func(tx *sql.Tx) error {
			if authenticated {
				id, err := s.accounts.SessionOwnerTx(ctx, tx, session.session.Key)
				if err != nil {
					return err
				}
				if id != owner.AccountID {
					return accounts.ErrInvalidSession
				}
			}
			if r.Method == http.MethodDelete {
				return notices.ResetTx(ctx, tx, owner)
			}
			return notices.SetHiddenTx(ctx, tx, owner, body.NoticeID, *body.Hidden, time.Now())
		})
		if err != nil {
			if errors.Is(err, accounts.ErrInvalidSession) {
				s.writeAuthenticationRequired(w)
			} else if errors.Is(err, notices.ErrUnknown) {
				writeError(w, http.StatusBadRequest, err)
			} else {
				writeError(w, http.StatusServiceUnavailable, errors.New("notice preferences could not be saved; refresh their status before retrying"))
			}
			return
		}
	}
	result, err := notices.Read(ctx, db.SQL(), owner, time.Now())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("notice preferences are temporarily unavailable"))
		return
	}
	writeBoundedJSON(w, http.StatusOK, result)
}

func (s *Server) requireNoticeScope(w http.ResponseWriter, r *http.Request, authenticated bool) bool {
	expected := r.Header.Get("X-MagicHandy-Notice-Scope")
	if expected == "" {
		return true
	}
	if expected == "account" && !authenticated {
		s.writeAuthenticationRequired(w)
		return false
	}
	if expected != "account" && expected != "browser" {
		writeError(w, http.StatusBadRequest, errors.New("invalid notice preference scope"))
		return false
	}
	if expected == "browser" && authenticated {
		writeError(w, http.StatusConflict, errors.New("notice preferences changed account; refresh to continue"))
		return false
	}
	return true
}

type noticePreferenceChange struct {
	NoticeID string `json:"notice_id"`
	Hidden   *bool  `json:"hidden"`
}

func decodeNoticePreference(w http.ResponseWriter, r *http.Request) (noticePreferenceChange, bool) {
	var body noticePreferenceChange
	if r.Method != http.MethodPut {
		finishUnreadBody(w, r)
		return body, true
	}
	if !requireJSONRequest(w, r) {
		return body, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := decodeJSON(r, &body); err != nil {
		rejectRequest(w, r, http.StatusBadRequest, err)
		return body, false
	}
	_ = controller.SetReadDeadline(time.Time{})
	if body.Hidden == nil {
		writeError(w, http.StatusBadRequest, errors.New("a notice visibility choice is required"))
		return body, false
	}
	return body, true
}

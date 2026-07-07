package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

func (s *Server) registerUIPreferenceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ui/preferences", s.handleUIPreferencesGet)
	mux.HandleFunc("PUT /api/ui/preferences", s.handleUIPreferencesPut)
}

func (s *Server) handleUIPreferencesGet(w http.ResponseWriter, _ *http.Request) {
	prefs, err := s.store.DB().LoadUIPreferences()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":                  prefs.Locale,
		"locale_prompt_dismissed": prefs.LocalePromptDismissed,
		"supported_locales":       store.SupportedLocales,
	})
}

func (s *Server) handleUIPreferencesPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Locale                string `json:"locale"`
		LocalePromptDismissed *bool  `json:"locale_prompt_dismissed"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current, err := s.store.DB().LoadUIPreferences()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	next := current
	if body.Locale != "" {
		next.Locale = body.Locale
	}
	if body.LocalePromptDismissed != nil {
		next.LocalePromptDismissed = *body.LocalePromptDismissed
	}
	saved, err := s.store.DB().SaveUIPreferences(next)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":                  saved.Locale,
		"locale_prompt_dismissed": saved.LocalePromptDismissed,
		"supported_locales":       store.SupportedLocales,
	})
}

package config

import (
	"fmt"
	"strings"
)

const (
	// UserGenderMale is the male gender option.
	UserGenderMale = "male"
	// UserGenderFemale is the female gender option.
	UserGenderFemale = "female"
	// UserGenderOther is the non-binary/other gender option.
	UserGenderOther = "other"
	// UserGenderUnspecified means the user prefers not to say.
	UserGenderUnspecified = "unspecified"

	// UserOrientationHeterosexual is the heterosexual orientation option.
	UserOrientationHeterosexual = "heterosexual"
	// UserOrientationHomosexual is the homosexual orientation option.
	UserOrientationHomosexual = "homosexual"
	// UserOrientationBisexual is the bisexual orientation option.
	UserOrientationBisexual = "bisexual"
	// UserOrientationPansexual is the pansexual orientation option.
	UserOrientationPansexual = "pansexual"
	// UserOrientationAsexual is the asexual orientation option.
	UserOrientationAsexual = "asexual"
	// UserOrientationOther is the other orientation option.
	UserOrientationOther = "other"
	// UserOrientationUnspecified means the user prefers not to say.
	UserOrientationUnspecified = "unspecified"

	maxUserAboutMeLength = 500
)

// UserProfileSettings stores how the user wants to be described in AI replies.
type UserProfileSettings struct {
	Gender            string `json:"gender"`
	SexualOrientation string `json:"sexual_orientation"`
	AboutMe           string `json:"about_me,omitempty"`
}

// Configured reports whether any user profile field is set for prompt injection.
func (p UserProfileSettings) Configured() bool {
	if strings.TrimSpace(p.AboutMe) != "" {
		return true
	}
	if gender := strings.TrimSpace(p.Gender); gender != "" && gender != UserGenderUnspecified {
		return true
	}
	if orientation := strings.TrimSpace(p.SexualOrientation); orientation != "" && orientation != UserOrientationUnspecified {
		return true
	}
	return false
}

func normalizeUserProfileSettings(settings UserProfileSettings) UserProfileSettings {
	settings.Gender = strings.TrimSpace(settings.Gender)
	if settings.Gender == "" {
		settings.Gender = UserGenderUnspecified
	}
	settings.SexualOrientation = strings.TrimSpace(settings.SexualOrientation)
	if settings.SexualOrientation == "" {
		settings.SexualOrientation = UserOrientationUnspecified
	}
	settings.AboutMe = strings.TrimSpace(settings.AboutMe)
	if len(settings.AboutMe) > maxUserAboutMeLength {
		settings.AboutMe = settings.AboutMe[:maxUserAboutMeLength]
	}
	return settings
}

func validateUserProfileSettings(settings UserProfileSettings) error {
	if !oneOf(
		settings.Gender,
		UserGenderMale,
		UserGenderFemale,
		UserGenderOther,
		UserGenderUnspecified,
	) {
		return fmt.Errorf("unknown user gender %q", settings.Gender)
	}
	if !oneOf(
		settings.SexualOrientation,
		UserOrientationHeterosexual,
		UserOrientationHomosexual,
		UserOrientationBisexual,
		UserOrientationPansexual,
		UserOrientationAsexual,
		UserOrientationOther,
		UserOrientationUnspecified,
	) {
		return fmt.Errorf("unknown user sexual orientation %q", settings.SexualOrientation)
	}
	if len(settings.AboutMe) > maxUserAboutMeLength {
		return fmt.Errorf("about_me must be at most %d characters", maxUserAboutMeLength)
	}
	return nil
}

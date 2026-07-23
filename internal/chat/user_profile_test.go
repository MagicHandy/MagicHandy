package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestUserProfileInstructionsPTBR(t *testing.T) {
	profile := config.UserProfileSettings{
		Gender:            config.UserGenderMale,
		SexualOrientation: config.UserOrientationHeterosexual,
		AboutMe:           "Gosto de provocação lenta.",
	}
	block := UserProfileInstructions(profile, PromptSetIDAutoDomV1PTBR)
	if !strings.Contains(block, "homem") {
		t.Fatalf("block = %q, want masculine gender instruction", block)
	}
	if !strings.Contains(block, "heterossexual") {
		t.Fatalf("block = %q, want heterosexual orientation", block)
	}
	if !strings.Contains(block, "Gosto de provocação lenta.") {
		t.Fatalf("block = %q, want about_me text", block)
	}
}

func TestUserProfileInstructionsSkipsUnspecified(t *testing.T) {
	profile := config.UserProfileSettings{
		Gender:            config.UserGenderUnspecified,
		SexualOrientation: config.UserOrientationUnspecified,
	}
	if block := UserProfileInstructions(profile, PromptSetIDPortugueseBrazil); block != "" {
		t.Fatalf("block = %q, want empty for unspecified profile", block)
	}
}

func TestAppendUserProfilePreservesSystemPrompt(t *testing.T) {
	base := "system text"
	profile := config.UserProfileSettings{Gender: config.UserGenderFemale}
	got := AppendUserProfile(base, profile, PromptSetIDPortugueseBrazil)
	if !strings.HasPrefix(got, base+"\n\n") {
		t.Fatalf("got = %q, want base prompt preserved", got)
	}
	if !strings.Contains(got, "mulher") {
		t.Fatalf("got = %q, want feminine instruction", got)
	}
}

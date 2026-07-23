package chat

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// AppendUserProfile appends user identity instructions when profile fields are set.
func AppendUserProfile(systemPrompt string, profile config.UserProfileSettings, promptSetID string) string {
	block := UserProfileInstructions(profile, promptSetID)
	if block == "" {
		return systemPrompt
	}
	return strings.TrimSpace(systemPrompt) + "\n\n" + block
}

// UserProfileInstructions builds prompt text for the configured user profile.
func UserProfileInstructions(profile config.UserProfileSettings, promptSetID string) string {
	if !profile.Configured() {
		return ""
	}
	switch strings.TrimSpace(promptSetID) {
	case PromptSetIDSpanish:
		return userProfileInstructionsES(profile)
	case PromptSetIDPortugueseBrazil, PromptSetIDAutoDomV1PTBR:
		return userProfileInstructionsPTBR(profile)
	case PromptSetIDSimplifiedChinese:
		return userProfileInstructionsZH(profile)
	case PromptSetIDJapanese:
		return userProfileInstructionsJA(profile)
	default:
		if strings.HasPrefix(strings.TrimSpace(promptSetID), "persona:") {
			return userProfileInstructionsPTBR(profile)
		}
		return userProfileInstructionsEN(profile)
	}
}

func userProfileInstructionsPTBR(profile config.UserProfileSettings) string {
	var lines []string
	lines = append(lines, "Perfil do usuário (use ao mencionar ou descrever o usuário; nunca contradiga estes dados):")
	if line := genderInstructionPTBR(profile.Gender); line != "" {
		lines = append(lines, "- "+line)
	}
	if line := orientationInstructionPTBR(profile.SexualOrientation); line != "" {
		lines = append(lines, "- "+line)
	}
	if about := strings.TrimSpace(profile.AboutMe); about != "" {
		lines = append(lines, "- Sobre mim: "+about)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func userProfileInstructionsEN(profile config.UserProfileSettings) string {
	var lines []string
	lines = append(lines, "User profile (use when mentioning or describing the user; never contradict these facts):")
	if line := genderInstructionEN(profile.Gender); line != "" {
		lines = append(lines, "- "+line)
	}
	if line := orientationInstructionEN(profile.SexualOrientation); line != "" {
		lines = append(lines, "- "+line)
	}
	if about := strings.TrimSpace(profile.AboutMe); about != "" {
		lines = append(lines, "- About me: "+about)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func userProfileInstructionsES(profile config.UserProfileSettings) string {
	var lines []string
	lines = append(lines, "Perfil del usuario (úsalo al mencionar o describir al usuario; nunca contradigas estos datos):")
	if line := genderInstructionES(profile.Gender); line != "" {
		lines = append(lines, "- "+line)
	}
	if line := orientationInstructionES(profile.SexualOrientation); line != "" {
		lines = append(lines, "- "+line)
	}
	if about := strings.TrimSpace(profile.AboutMe); about != "" {
		lines = append(lines, "- Sobre mí: "+about)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func userProfileInstructionsZH(profile config.UserProfileSettings) string {
	var lines []string
	lines = append(lines, "用户资料（提及或描述用户时使用；不得与以下信息矛盾）：")
	if line := genderInstructionZH(profile.Gender); line != "" {
		lines = append(lines, "- "+line)
	}
	if line := orientationInstructionZH(profile.SexualOrientation); line != "" {
		lines = append(lines, "- "+line)
	}
	if about := strings.TrimSpace(profile.AboutMe); about != "" {
		lines = append(lines, "- 关于我："+about)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func userProfileInstructionsJA(profile config.UserProfileSettings) string {
	var lines []string
	lines = append(lines, "ユーザープロフィール（ユーザーを言及・描写するときに使う。以下と矛盾させないこと）：")
	if line := genderInstructionJA(profile.Gender); line != "" {
		lines = append(lines, "- "+line)
	}
	if line := orientationInstructionJA(profile.SexualOrientation); line != "" {
		lines = append(lines, "- "+line)
	}
	if about := strings.TrimSpace(profile.AboutMe); about != "" {
		lines = append(lines, "- 自分について："+about)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func genderInstructionPTBR(gender string) string {
	switch gender {
	case config.UserGenderMale:
		return "Gênero: homem — use pronomes e adjetivos no masculino (ele, dele, seu) ao falar do usuário"
	case config.UserGenderFemale:
		return "Gênero: mulher — use pronomes e adjetivos no feminino (ela, dela, sua) ao falar do usuário"
	case config.UserGenderOther:
		return "Gênero: outro/não-binário — evite assumir gênero; use linguagem inclusiva ou neutra"
	default:
		return ""
	}
}

func genderInstructionEN(gender string) string {
	switch gender {
	case config.UserGenderMale:
		return "Gender: man — use masculine pronouns and adjectives (he, him, his) when referring to the user"
	case config.UserGenderFemale:
		return "Gender: woman — use feminine pronouns and adjectives (she, her, hers) when referring to the user"
	case config.UserGenderOther:
		return "Gender: other/non-binary — do not assume gender; use inclusive or neutral language"
	default:
		return ""
	}
}

func genderInstructionES(gender string) string {
	switch gender {
	case config.UserGenderMale:
		return "Género: hombre — usa pronombres y adjetivos masculinos (él, de él, su) al hablar del usuario"
	case config.UserGenderFemale:
		return "Género: mujer — usa pronombres y adjetivos femeninos (ella, de ella, su) al hablar del usuario"
	case config.UserGenderOther:
		return "Género: otro/no binario — no asumas género; usa lenguaje inclusivo o neutro"
	default:
		return ""
	}
}

func genderInstructionZH(gender string) string {
	switch gender {
	case config.UserGenderMale:
		return "性别：男性 — 提及用户时使用男性代词与表述"
	case config.UserGenderFemale:
		return "性别：女性 — 提及用户时使用女性代词与表述"
	case config.UserGenderOther:
		return "性别：其他/非二元 — 不要假设性别，使用包容或中性表述"
	default:
		return ""
	}
}

func genderInstructionJA(gender string) string {
	switch gender {
	case config.UserGenderMale:
		return "性別：男性 — ユーザーを言及するときは男性の人称・表現を使う"
	case config.UserGenderFemale:
		return "性別：女性 — ユーザーを言及するときは女性の人称・表現を使う"
	case config.UserGenderOther:
		return "性別：その他/ノンバイナリー — 性別を決めつけず、包括的または中立的な表現を使う"
	default:
		return ""
	}
}

func orientationInstructionPTBR(orientation string) string {
	label := orientationLabelPTBR(orientation)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("Orientação sexual: %s — considere isso ao mencionar desejo, atração ou contexto íntimo do usuário", label)
}

func orientationInstructionEN(orientation string) string {
	label := orientationLabelEN(orientation)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("Sexual orientation: %s — consider this when mentioning the user's desire, attraction, or intimate context", label)
}

func orientationInstructionES(orientation string) string {
	label := orientationLabelES(orientation)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("Orientación sexual: %s — tenlo en cuenta al mencionar deseo, atracción o contexto íntimo del usuario", label)
}

func orientationInstructionZH(orientation string) string {
	label := orientationLabelZH(orientation)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("性取向：%s — 提及用户欲望、吸引或亲密语境时请考虑这一点", label)
}

func orientationInstructionJA(orientation string) string {
	label := orientationLabelJA(orientation)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("性的指向：%s — ユーザーの欲望・魅力・親密な文脈に言及するときはこれを考慮する", label)
}

func orientationLabelPTBR(orientation string) string {
	switch orientation {
	case config.UserOrientationHeterosexual:
		return "heterossexual"
	case config.UserOrientationHomosexual:
		return "homossexual"
	case config.UserOrientationBisexual:
		return "bissexual"
	case config.UserOrientationPansexual:
		return "pansexual"
	case config.UserOrientationAsexual:
		return "assexual"
	case config.UserOrientationOther:
		return "outra"
	default:
		return ""
	}
}

func orientationLabelEN(orientation string) string {
	switch orientation {
	case config.UserOrientationHeterosexual:
		return "heterosexual"
	case config.UserOrientationHomosexual:
		return "homosexual"
	case config.UserOrientationBisexual:
		return "bisexual"
	case config.UserOrientationPansexual:
		return "pansexual"
	case config.UserOrientationAsexual:
		return "asexual"
	case config.UserOrientationOther:
		return "other"
	default:
		return ""
	}
}

func orientationLabelES(orientation string) string {
	switch orientation {
	case config.UserOrientationHeterosexual:
		return "heterosexual"
	case config.UserOrientationHomosexual:
		return "homosexual"
	case config.UserOrientationBisexual:
		return "bisexual"
	case config.UserOrientationPansexual:
		return "pansexual"
	case config.UserOrientationAsexual:
		return "asexual"
	case config.UserOrientationOther:
		return "otra"
	default:
		return ""
	}
}

func orientationLabelZH(orientation string) string {
	switch orientation {
	case config.UserOrientationHeterosexual:
		return "异性恋"
	case config.UserOrientationHomosexual:
		return "同性恋"
	case config.UserOrientationBisexual:
		return "双性恋"
	case config.UserOrientationPansexual:
		return "泛性恋"
	case config.UserOrientationAsexual:
		return "无性恋"
	case config.UserOrientationOther:
		return "其他"
	default:
		return ""
	}
}

func orientationLabelJA(orientation string) string {
	switch orientation {
	case config.UserOrientationHeterosexual:
		return "ヘテロセクシャル"
	case config.UserOrientationHomosexual:
		return "ホモセクシャル"
	case config.UserOrientationBisexual:
		return "バイセクシャル"
	case config.UserOrientationPansexual:
		return "パンセクシャル"
	case config.UserOrientationAsexual:
		return "アセクシャル"
	case config.UserOrientationOther:
		return "その他"
	default:
		return ""
	}
}

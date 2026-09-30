package chat

// ReplyLength steers how long the "reply" field runs. It shapes prose only:
// the motion contract and every motion gate are identical at every length.
type ReplyLength string

const (
	// ReplyLengthBalanced is the zero value and composes nothing, so the
	// prompts keep the pacing they were tuned with.
	ReplyLengthBalanced ReplyLength = ""
	// ReplyLengthShort asks for one or two short sentences.
	ReplyLengthShort ReplyLength = "short"
	// ReplyLengthDetailed allows three to five sentences when the moment
	// calls for it.
	ReplyLengthDetailed ReplyLength = "detailed"
)

// replyLengthInstructionForLocale is the instruction composed after the
// final voice check, in the prompt's own language; empty for balanced.
func replyLengthInstructionForLocale(locale promptLocale, length ReplyLength) string {
	switch length {
	case ReplyLengthShort:
		switch locale {
		case promptLocaleSpanish:
			return `Limita el campo "reply" a una o dos frases cortas.`
		case promptLocalePortugueseBrazil:
			return `Mantenha o campo "reply" em uma ou duas frases curtas.`
		case promptLocaleSimplifiedChinese:
			return `"reply" 字段只写一到两个短句。`
		case promptLocaleJapanese:
			return `"reply" フィールドは短い一文か二文にしてください。`
		default:
			return `Keep the "reply" field to one or two short sentences.`
		}
	case ReplyLengthDetailed:
		switch locale {
		case promptLocaleSpanish:
			return `El campo "reply" puede tener de tres a cinco frases cuando el momento lo pida; mantén la misma voz.`
		case promptLocalePortugueseBrazil:
			return `O campo "reply" pode ter de três a cinco frases quando o momento pedir; mantenha a mesma voz.`
		case promptLocaleSimplifiedChinese:
			return `在合适的时候，"reply" 字段可以写三到五句；保持同样的语气。`
		case promptLocaleJapanese:
			return `必要な場面では、"reply" フィールドを三〜五文にして構いません。口調は変えないでください。`
		default:
			return `The "reply" field may run three to five sentences when the moment calls for it; keep the same voice.`
		}
	default:
		return ""
	}
}

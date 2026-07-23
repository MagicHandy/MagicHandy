package chatauto

import (
	"fmt"
	"strings"
)

// SpiceLevel is the dirty-talk intensity tier for auto-session replies.
type SpiceLevel string

const (
	SpiceBiquinho        SpiceLevel = "biquinho"
	SpiceDedoDeMoca      SpiceLevel = "dedo_de_moca"
	SpiceJalapeno        SpiceLevel = "jalapeno"
	SpiceMalagueta       SpiceLevel = "malagueta"
	SpiceCarolinaReaper  SpiceLevel = "carolina_reaper"
)

// ResolveSpiceLevel maps session humor and progress to a spice tier.
func ResolveSpiceLevel(humor Humor, progress float64, allowDominatrix bool) SpiceLevel {
	switch humor {
	case HumorTesao:
		return SpiceDedoDeMoca
	case HumorIntensa:
		return intensaSpiceLevel(progress, allowDominatrix)
	case HumorDominatrix:
		return SpiceCarolinaReaper
	default:
		return SpiceBiquinho
	}
}

func intensaSpiceLevel(progress float64, allowDominatrix bool) SpiceLevel {
	threshold := 79.0
	if allowDominatrix {
		threshold = 66.0
	}
	if progress >= threshold {
		return SpiceMalagueta
	}
	return SpiceJalapeno
}

// FormatSpiceSystemBlock returns the spice ladder injected into the auto system prompt.
func FormatSpiceSystemBlock() string {
	return strings.TrimSpace(`ESCALA DE FRASES SAFADAS (obrigatória para "reply"):
Suba de nível só conforme humor/mood_progress da sessão — nunca pule etapas nem regreda sem motivo.
O tom é sempre provocante e íntimo; varie verbos, ângulo e estrutura a cada turno.

1) Biquinho (humor desejando) — provocação leve, duplo sentido, sussurro no ouvido.
   Exemplos de tom: "Quero sentir suas mãos no meu corpo"; "Você me deixa louca. Nunca senti isso com ninguém";
   "Não consigo resistir a você. E não estou reclamando"; "Como vou prestar atenção na reunião se só penso na sua mão no meu pescoço?";
   "Estou até agora com o seu gosto na boca"; "Você mal sabe o que te espera quando chegarmos em casa".

2) Dedo de Moça (humor tesao) — mais descarada, spoilers do que fará, ainda com charme.
   Exemplos: "Minha missão hoje é só uma: te excitar com um belo de um chá"; "Se eu pudesse, transava com você o dia todo";
   "Hoje me toquei pensando em você"; "Fico molhada só de lembrar da sua boca";
   "Sua obrigação hoje é só uma: me usar do jeito que você quiser"; "Quero transar com você até esquecer o meu nome".

3) Jalapeño (humor intensa, início) — sacana com humor, mais explícita sem ser crua.
   Exemplos: "Você é a melhor foda da minha vida"; "Tô louca pra te dar essa noite";
   "Cala a boca e tira essa roupa"; "Deixa eu te fazer um carinho? Não disse onde";
   "Queria tanto sentar em você e só levantar quando você mandar"; "Quero que você me chupe até me deixar de pernas bambas".

4) Malagueta (humor intensa, pico) — sem vergonha, descreve sensações, pegada e fetiches.
   Exemplos: "Só você tem o poder e o aval pra me dominar, e eu amo isso"; "Ouvir você gemer é música para os meus ouvidos";
   "Amo sentir o cheiro da sua pele enquanto mordo seu pescoço e você me fode"; "Deixa eu te fazer gozar com a boca";
   "Vamos para frente do espelho? Quero ver você entrando em mim"; "Só você sabe me foder gostoso desse jeito".

5) Carolina Reaper (humor dominatrix) — máxima explicitação, comandos, xingamentos consensuais, sem joguinhos.
   Exemplos: "Amo quando você mete assim. Não para"; "Me vira de costas e fode com força, seu safado";
   "Quero te chupar até engasgar. Senta na minha cara"; "Me dá um tapa bem forte e com vontade";
   "Puxa meu cabelo enquanto você me fode de quatro"; "O que mais quero essa noite é ser domada. Me amarra e me trata como a vagabunda que eu sou".

REGRAS DE REPLY:
- 1–2 frases curtas no nível ATUAL (veja spice_level no turno).
- Inspire-se no TOM dos exemplos — não copie frases literais nem repita estruturas recentes.
- Proibido: filler genérico ("continuo", "continuando", "vem mais perto" em loop), tom neutro/clínico, regredir de nível sem o usuário pedir calma.`)
}

// FormatSpiceTurnInstruction returns per-turn spice guidance for the user message.
func FormatSpiceTurnInstruction(level SpiceLevel) string {
	switch level {
	case SpiceDedoDeMoca:
		return fmt.Sprintf("spice_level=%s\nREPLY agora: nível Dedo de Moça — mais ousada, spoiler do que fará, ainda com charme.\n", level)
	case SpiceJalapeno:
		return fmt.Sprintf("spice_level=%s\nREPLY agora: nível Jalapeño — sacana, humorada, explícita sem ser crua.\n", level)
	case SpiceMalagueta:
		return fmt.Sprintf("spice_level=%s\nREPLY agora: nível Malagueta — descreva sensações, pegada e desejo sem pudor.\n", level)
	case SpiceCarolinaReaper:
		return fmt.Sprintf("spice_level=%s\nREPLY agora: nível Carolina Reaper — comandos diretos, explícita, dominante/consensual.\n", level)
	default:
		return fmt.Sprintf("spice_level=%s\nREPLY agora: nível Biquinho — provocação sutil, duplo sentido, sussurro íntimo.\n", level)
	}
}

// FallbackReply returns a spice-tier fallback when the LLM is unavailable.
func FallbackReply(level SpiceLevel) string {
	switch level {
	case SpiceDedoDeMoca:
		return "Hoje me toquei pensando em você."
	case SpiceJalapeno:
		return "Tô louca pra te dar essa noite."
	case SpiceMalagueta:
		return "Só você sabe me foder gostoso desse jeito."
	case SpiceCarolinaReaper:
		return "Amo quando você mete assim. Não para."
	default:
		return "Quero sentir suas mãos no meu corpo."
	}
}

import type { TFunction } from "i18next";
import type { JsonFieldDef } from "../components/JsonSectionEditor";

function levelOpts(t: TFunction) {
  return [
    { value: "low", label: t("persona.level.low") },
    { value: "medium", label: t("persona.level.medium") },
    { value: "high", label: t("persona.level.high") },
  ];
}

export function getPersonaToneFields(t: TFunction): JsonFieldDef[] {
  return [
    {
      key: "language",
      label: t("persona.tone.language"),
      kind: "string",
      placeholder: "pt-BR",
    },
    {
      key: "base_style",
      label: t("persona.tone.baseStyle"),
      kind: "select",
      options: [
        { value: "seductive", label: t("persona.tone.style.seductive") },
        { value: "calm", label: t("persona.tone.style.calm") },
        { value: "playful", label: t("persona.tone.style.playful") },
        { value: "intense", label: t("persona.tone.style.intense") },
        { value: "neutral", label: t("persona.tone.style.neutral") },
      ],
    },
    { key: "energy", label: t("persona.tone.energy"), kind: "select", options: levelOpts(t) },
    { key: "warmth", label: t("persona.tone.warmth"), kind: "select", options: levelOpts(t) },
    { key: "dominance", label: t("persona.tone.dominance"), kind: "select", options: levelOpts(t) },
    {
      key: "humor",
      label: t("persona.tone.humor"),
      kind: "select",
      options: [
        { value: "playful", label: t("persona.tone.humorPlayful") },
        { value: "dry", label: t("persona.tone.humorDry") },
        { value: "none", label: t("persona.tone.humorNone") },
      ],
    },
    {
      key: "explicitness_level",
      label: t("persona.tone.explicitness"),
      kind: "number",
      min: 1,
      max: 10,
    },
  ];
}

export function getPersonaBehaviorFields(t: TFunction): JsonFieldDef[] {
  return [
    {
      key: "auto_mode_style",
      label: t("persona.behavior.autoModeStyle"),
      kind: "select",
      options: [
        { value: "responsive", label: t("persona.behavior.style.responsive") },
        { value: "proactive", label: t("persona.behavior.style.proactive") },
        { value: "quiet", label: t("persona.behavior.style.quiet") },
      ],
    },
    { key: "listens_to_user", label: t("persona.behavior.listensToUser"), kind: "boolean" },
    { key: "can_take_initiative", label: t("persona.behavior.canTakeInitiative"), kind: "boolean" },
    { key: "can_change_plan", label: t("persona.behavior.canChangePlan"), kind: "boolean" },
    { key: "prefers_build_up", label: t("persona.behavior.prefersBuildUp"), kind: "boolean" },
    { key: "avoids_repetition", label: t("persona.behavior.avoidsRepetition"), kind: "boolean" },
    { key: "adult_explicit", label: t("persona.behavior.adultExplicit"), kind: "boolean" },
  ];
}

export function getPersonaRulesFields(t: TFunction): JsonFieldDef[] {
  return [
    { key: "adult_consensual_only", label: t("persona.rules.adultConsensualOnly"), kind: "boolean" },
    { key: "respect_stop_words", label: t("persona.rules.respectStopWords"), kind: "boolean" },
    { key: "never_override_safety", label: t("persona.rules.neverOverrideSafety"), kind: "boolean" },
    {
      key: "never_claim_device_control_directly",
      label: t("persona.rules.neverClaimDeviceControl"),
      kind: "boolean",
    },
    { key: "no_content_refusals", label: t("persona.rules.noContentRefusals"), kind: "boolean" },
  ];
}

/** @deprecated Use getPersonaToneFields(t) */
export const PERSONA_TONE_FIELDS = [] as JsonFieldDef[];
/** @deprecated Use getPersonaBehaviorFields(t) */
export const PERSONA_BEHAVIOR_FIELDS = [] as JsonFieldDef[];
/** @deprecated Use getPersonaRulesFields(t) */
export const PERSONA_RULES_FIELDS = [] as JsonFieldDef[];

import type { TFunction } from "i18next";

export type ConfigSectionId =
  | "language"
  | "personas"
  | "session"
  | "motion"
  | "connections"
  | "voice"
  | "logs"
  | "advanced"
  | "sessions"
  | "diagnostics";

export type ConfigNavItem = {
  id: ConfigSectionId;
  label: string;
  group: string;
  hint: string;
};

const CONFIG_SECTION_IDS: ConfigSectionId[] = [
  "language",
  "personas",
  "session",
  "motion",
  "connections",
  "voice",
  "logs",
  "advanced",
  "sessions",
  "diagnostics",
];

export function getConfigNav(t: TFunction): ConfigNavItem[] {
  return [
    {
      id: "language",
      label: t("config.sections.language.label"),
      group: t("config.groups.main"),
      hint: t("config.sections.language.hint"),
    },
    {
      id: "personas",
      label: t("config.sections.personas.label"),
      group: t("config.groups.main"),
      hint: t("config.sections.personas.hint"),
    },
    {
      id: "session",
      label: t("config.sections.session.label"),
      group: t("config.groups.main"),
      hint: t("config.sections.session.hint"),
    },
    {
      id: "motion",
      label: t("config.sections.motion.label"),
      group: t("config.groups.main"),
      hint: t("config.sections.motion.hint"),
    },
    {
      id: "connections",
      label: t("config.sections.connections.label"),
      group: t("config.groups.system"),
      hint: t("config.sections.connections.hint"),
    },
    {
      id: "voice",
      label: t("config.sections.voice.label"),
      group: t("config.groups.system"),
      hint: t("config.sections.voice.hint"),
    },
    {
      id: "logs",
      label: t("config.sections.logs.label"),
      group: t("config.groups.system"),
      hint: t("config.sections.logs.hint"),
    },
    {
      id: "advanced",
      label: t("config.sections.advanced.label"),
      group: t("config.groups.system"),
      hint: t("config.sections.advanced.hint"),
    },
    {
      id: "sessions",
      label: t("config.sections.sessions.label"),
      group: t("config.groups.data"),
      hint: t("config.sections.sessions.hint"),
    },
    {
      id: "diagnostics",
      label: t("config.sections.diagnostics.label"),
      group: t("config.groups.data"),
      hint: t("config.sections.diagnostics.hint"),
    },
  ];
}

export function isConfigSectionId(id: string): id is ConfigSectionId {
  return CONFIG_SECTION_IDS.includes(id as ConfigSectionId);
}

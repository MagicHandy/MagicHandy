import definitions from "../../internal/notices/catalog.json";

// One catalog is consumed by the backend validation and the presentation layer.
export const noticeDefinitions = definitions;
export type NoticePreferences = { scope: "account" | "browser"; hidden: string[] };
export const noticeDefinition = (id: string) => noticeDefinitions.find(entry => entry.id === id);

export function validateNoticePreferences(value: NoticePreferences): NoticePreferences {
  if (!value || !["account", "browser"].includes(value.scope) || !Array.isArray(value.hidden)
    || value.hidden.length > noticeDefinitions.length || value.hidden.some(id => typeof id !== "string" || !noticeDefinition(id))) {
    throw new Error("Invalid notice preferences response.");
  }
  return { scope: value.scope, hidden: [...new Set(value.hidden)] };
}

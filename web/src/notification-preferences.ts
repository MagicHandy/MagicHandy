import type { NotificationCategory } from "./api/types";

export const DEFAULT_NOTIFICATION_CATEGORIES: NotificationCategory[] = ["system", "voice", "updates"];

export function notificationCategories(value?: NotificationCategory[]): NotificationCategory[] {
  return value ?? DEFAULT_NOTIFICATION_CATEGORIES;
}

import { t } from "../i18n";

export interface ConnectionMethod {
  value: string;
  label: string;
  /** The method's name inside a status line. */
  short: string;
  detail: string;
}

/** The Handy's connection methods, in the order Setup and its device page offer them. */
export function handyConnectionMethods(): ConnectionMethod[] {
  return [
    { value: "cloud_rest", label: t("Handy Cloud"), short: t("Cloud"), detail: t("Recommended for firmware 4. Needs your connection key and internet access.") },
    { value: "native_bluetooth", label: t("Bluetooth on this computer"), short: t("Bluetooth on this computer"), detail: t("No key and no browser pairing. Windows and Linux.") },
    { value: "browser_bluetooth", label: t("Browser Bluetooth"), short: t("Browser Bluetooth"), detail: t("Pairs from a compatible browser on this device.") },
    { value: "intiface", label: t("Intiface Central"), short: t("Intiface Central"), detail: t("Uses the shared Intiface Central connection.") },
  ];
}

export function handyMethod(owner: string): ConnectionMethod | undefined {
  return handyConnectionMethods().find((method) => method.value === owner);
}

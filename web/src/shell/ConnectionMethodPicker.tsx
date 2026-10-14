import { useId } from "react";
import { t } from "../i18n";
import { useMenu } from "../components/useMenu";
import { CheckIcon, ChevronUpIcon } from "./icons";

export interface ConnectionMethod {
  value: string;
  label: string;
  detail: string;
}

/** The Handy's connection methods, in Setup's order and with Setup's explanations. */
export function handyConnectionMethods(): ConnectionMethod[] {
  return [
    { value: "cloud_rest", label: t("Handy Cloud REST"), detail: t("Recommended for firmware v4. Requires your private connection key and internet access.") },
    { value: "browser_bluetooth", label: t("Browser Bluetooth"), detail: t("Connect directly from a compatible browser. No cloud connection key is stored.") },
    { value: "native_bluetooth", label: t("Native Bluetooth"), detail: t("Connect through the computer running MagicHandy. No browser pairing or cloud key required.") },
    { value: "intiface", label: t("Intiface Central"), detail: t("Use an existing Intiface Central session for local device control.") },
  ];
}

interface Props {
  /** The visible device title that names this choice. */
  titleId: string;
  value: string;
  /** Shown when the saved value is not one of the methods. */
  fallbackLabel: string;
  methods: ConnectionMethod[];
  busy: boolean;
  onChange: (value: string) => void;
}

/**
 * The manager title's method list. It is a menu button rather than a native
 * select so arrow keys browse the choices without switching the transport;
 * only an explicit choice applies. The label always shows the saved value.
 */
export function ConnectionMethodPicker({ titleId, value, fallbackLabel, methods, busy, onChange }: Props) {
  const menu = useMenu({ disabled: busy });
  const nameId = useId();
  const hintId = useId();
  const current = methods.find((method) => method.value === value);

  return (
    <div className="connection-method">
      <button
        type="button"
        className="connection-method-trigger"
        aria-haspopup="menu"
        aria-expanded={menu.open}
        aria-controls={menu.open ? menu.id : undefined}
        aria-labelledby={`${titleId} ${nameId}`}
        aria-busy={busy || undefined}
        onClick={(event) => {
          if (menu.open) menu.close();
          else if (!busy) menu.show(event.currentTarget, undefined);
        }}
        onKeyDown={(event) => { if (!busy) menu.onTriggerKeyDown(event, undefined); }}
      >
        <span id={nameId} className="connection-method-name">{current?.label ?? fallbackLabel}</span>
        <ChevronUpIcon size={14} className="connection-method-chevron" />
      </button>
      {menu.open && (
        <div {...menu.menuProps} className="connection-method-menu" aria-labelledby={titleId} aria-describedby={hintId}>
          {methods.map((method) => {
            const optionId = `${hintId}-${method.value}`;
            return (
              <button
                key={method.value}
                type="button"
                role="menuitemradio"
                aria-checked={method.value === value}
                aria-labelledby={`${optionId}-name`}
                aria-describedby={`${optionId}-detail`}
                className="connection-method-option"
                onClick={() => {
                  menu.close();
                  if (method.value !== value) onChange(method.value);
                }}
              >
                <span className="connection-method-option-copy">
                  <strong id={`${optionId}-name`}>{method.label}</strong>
                  <small id={`${optionId}-detail`}>{method.detail}</small>
                </span>
                {method.value === value && <CheckIcon size={16} className="connection-method-check" />}
              </button>
            );
          })}
          <p id={hintId} className="connection-method-hint">{t("Changing it stops motion.")}</p>
        </div>
      )}
    </div>
  );
}

import { t } from "../i18n";
import { availableThemes, DEFAULT_THEME, normalizeTheme, type UIThemeID } from "../theme";

interface ThemePickerProps {
  value?: string;
  allowedThemes?: readonly string[];
  disabled?: boolean;
  onChange: (theme: UIThemeID) => void;
}

export function ThemePicker({
  value,
  allowedThemes,
  disabled = false,
  onChange,
}: ThemePickerProps) {
  const selected = normalizeTheme(value);
  const themes = availableThemes(allowedThemes);

  return (
    <fieldset className="theme-picker" disabled={disabled}>
      <legend className="label">{t("Theme")}</legend>
      <div className="theme-choice-list" data-last-row={themes.length % 4}>
        {themes.map((theme) => (
          <label
            className="theme-choice"
            data-selected={selected === theme.id}
            key={theme.id}
          >
            <input
              type="radio"
              name="ui-theme"
              value={theme.id}
              checked={selected === theme.id}
              onChange={() => onChange(theme.id as UIThemeID)}
            />
            <span className="theme-swatches" aria-hidden="true">
              {theme.swatches.map((color) => (
                <span key={color} style={{ backgroundColor: color }} />
              ))}
            </span>
            <span className="theme-choice-name">{theme.label}</span>
            {theme.id === DEFAULT_THEME && <span className="theme-default">{t("Default")}</span>}
          </label>
        ))}
      </div>
      <span className="hint-block">
        {t("The saved theme is shared by every open tab and applies after Save settings.")}
      </span>
    </fieldset>
  );
}

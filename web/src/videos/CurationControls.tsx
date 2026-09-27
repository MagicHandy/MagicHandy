import { formatNumber, t } from "../i18n";
import { useState } from "react";
import { CloseIcon, StarIcon } from "../shell/icons";
import { canonicalTag, splitTagInput } from "./curation";

const STARS = [1, 2, 3, 4, 5] as const;

/** Read-only stars for a rated video; renders nothing when unrated. */
export function RatingStars({ rating }: { rating?: number | null }) {
  if (!rating) return null;
  return (
    <span className="video-rating" role="img" aria-label={t("Rated {count} of 5", { count: rating })}>
      {STARS.map((star) => <StarIcon key={star} size={12} filled={star <= rating} />)}
    </span>
  );
}

interface RatingInputProps {
  value: number;
  disabled?: boolean;
  onChange: (value: number) => void;
}

/** Choosing the current rating again clears it. */
export function RatingInput({ value, disabled = false, onChange }: RatingInputProps) {
  return (
    <div className="video-rating-input" role="group" aria-label={t("Rating")}>
      {STARS.map((star) => (
        <button
          key={star}
          type="button"
          className="icon-button video-star"
          aria-pressed={value === star}
          aria-label={t("Rate {count} of 5", { count: star })}
          title={t("Rate {count} of 5", { count: star })}
          data-filled={star <= value || undefined}
          disabled={disabled}
          onClick={() => onChange(value === star ? 0 : star)}
        >
          <StarIcon size={18} filled={star <= value} />
        </button>
      ))}
      {value > 0 && <button type="button" className="btn btn-secondary compact-command" disabled={disabled} onClick={() => onChange(0)}>{t("Clear rating")}</button>}
    </div>
  );
}

/** Up to `limit` tags, with a count of the rest. */
export function TagChips({ tags, limit = 3 }: { tags?: string[]; limit?: number }) {
  if (!tags?.length) return null;
  const shown = tags.slice(0, limit);
  return (
    <span className="video-tags">
      {shown.map((tag) => <span key={tag} className="video-tag">{tag}</span>)}
      {tags.length > shown.length && <span className="video-tag video-tag-more" aria-label={t("{count} more tags", { count: tags.length - shown.length })}>+{formatNumber(tags.length - shown.length)}</span>}
    </span>
  );
}

interface TagEditorProps {
  inputID: string;
  tags: string[];
  suggestions: readonly string[];
  disabled?: boolean;
  onChange: (tags: string[]) => void;
}

// Enter, a comma or leaving the field adds the typed tag. A tag the library
// already has keeps the library's spelling.
export function TagEditor({ inputID, tags, suggestions, disabled = false, onChange }: TagEditorProps) {
  const [draft, setDraft] = useState("");
  const listID = `${inputID}-suggestions`;
  const lower = new Set(tags.map((tag) => tag.toLocaleLowerCase()));

  function commit(text: string) {
    const additions = splitTagInput(text);
    setDraft("");
    if (additions.length === 0) return;
    const next = [...tags];
    for (const addition of additions) {
      const tag = canonicalTag(addition, suggestions);
      if (next.some((existing) => existing.toLocaleLowerCase() === tag.toLocaleLowerCase())) continue;
      next.push(tag);
    }
    if (next.length !== tags.length) onChange(next);
  }

  return (
    <div className="video-tag-editor">
      {tags.length > 0 && (
        <ul className="video-tag-list" aria-label={t("Current tags")}>
          {tags.map((tag) => (
            <li key={tag.toLocaleLowerCase()} className="video-tag">
              <span>{tag}</span>
              <button type="button" aria-label={t("Remove tag {tag}", { tag })} title={t("Remove tag {tag}", { tag })} disabled={disabled} onClick={() => onChange(tags.filter((entry) => entry !== tag))}>
                <CloseIcon size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <input
        id={inputID}
        list={listID}
        value={draft}
        maxLength={200}
        disabled={disabled}
        placeholder={t("Add a tag")}
        onChange={(event) => {
          const value = event.target.value;
          if (value.includes(",")) commit(value);
          else setDraft(value);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            commit(draft);
          } else if (event.key === "Backspace" && draft === "" && tags.length > 0) {
            onChange(tags.slice(0, -1));
          }
        }}
        onBlur={() => commit(draft)}
      />
      <datalist id={listID}>
        {suggestions.filter((tag) => !lower.has(tag.toLocaleLowerCase())).map((tag) => <option key={tag} value={tag} />)}
      </datalist>
    </div>
  );
}

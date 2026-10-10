import { t, translateKnown } from "../i18n";
import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { ChatSession } from "../api/types";
import { MoreHorizontalIcon, PlusIcon, SaveIcon, TrashIcon } from "../shell/icons";
import { PersonaSwitcher } from "./PersonaSwitcher";
import { useMenu, type MenuFocus } from "./useMenu";

interface Props {
  sessions: ChatSession[];
  activeId: string;
  disabled: boolean;
  personaDisabled?: boolean;
  onActivate: (session: ChatSession) => void;
  onNew: () => void;
  onSave: (session: ChatSession) => void;
  onDelete: (session: ChatSession) => void;
  onPersonaChanged?: () => void;
  assistantMood?: string;
}

interface MenuTarget {
  session: ChatSession;
  left: number;
  top: number;
}

const PREFERRED_TAB_WIDTH = 236;
const TAB_GAP = 2;
const TAB_STRIP_INLINE_PADDING = 8;

function preferredTabStripWidth(tabCount: number) {
  return tabCount * PREFERRED_TAB_WIDTH
    + Math.max(0, tabCount - 1) * TAB_GAP
    + TAB_STRIP_INLINE_PADDING;
}

export function ChatTabs({
  sessions,
  activeId,
  disabled,
  personaDisabled = disabled,
  onActivate,
  onNew,
  onSave,
  onDelete,
  onPersonaChanged,
  assistantMood,
}: Props) {
  const menu = useMenu<MenuTarget>({ disabled });
  const activeRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const revealActiveTab = () => {
      activeRef.current?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    };
    revealActiveTab();

    const strip = scrollRef.current;
    if (!strip || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(revealActiveTab);
    observer.observe(strip);
    return () => observer.disconnect();
  }, [activeId]);

  useEffect(() => {
    const strip = scrollRef.current;
    if (!strip) return;
    const scrollOverflow = (event: WheelEvent) => {
      if (strip.scrollWidth <= strip.clientWidth) return;
      const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
      if (delta === 0) return;
      event.preventDefault();
      strip.scrollLeft += delta;
    };
    strip.addEventListener("wheel", scrollOverflow, { passive: false });
    return () => strip.removeEventListener("wheel", scrollOverflow);
  }, []);

  function openMenu(session: ChatSession, left: number, top: number, opener: HTMLElement, focus?: MenuFocus) {
    if (session.saved && session.active) {
      menu.close(false);
      return;
    }
    const menuWidth = 190;
    const menuHeight = 96;
    menu.show(opener, {
      session,
      left: Math.max(8, Math.min(left, window.innerWidth - menuWidth - 8)),
      top: Math.max(8, Math.min(top, window.innerHeight - menuHeight - 8)),
    }, focus);
  }

  function openFromButton(session: ChatSession, button: HTMLElement, focus?: MenuFocus) {
    const rect = button.getBoundingClientRect();
    openMenu(session, rect.right - 190, rect.bottom + 4, button, focus);
  }

  function menuButtonKeyDown(session: ChatSession, event: ReactKeyboardEvent<HTMLButtonElement>) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    openFromButton(session, event.currentTarget, event.key === "ArrowUp" ? "last" : "first");
  }

  const target = menu.value;

  // Saving the active chat removes its options button and deleting removes the
  // whole tab, so a chosen action hands focus to a tab that outlives it rather
  // than to the opener.
  function chooseAction(focusSessionID: string, action: () => void) {
    menu.close(false);
    document.getElementById(`chat-tab-${focusSessionID}`)?.focus();
    action();
  }

  function moveTabFocus(session: ChatSession, key: string) {
    const current = sessions.findIndex((candidate) => candidate.id === session.id);
    if (current < 0) return;
    let next = current;
    if (key === "ArrowRight") next = (current + 1) % sessions.length;
    else if (key === "ArrowLeft") next = (current - 1 + sessions.length) % sessions.length;
    else if (key === "Home") next = 0;
    else if (key === "End") next = sessions.length - 1;
    else return;
    const nextTab = document.getElementById(`chat-tab-${sessions[next].id}`);
    nextTab?.focus();
    nextTab?.closest(".chat-tab-wrap")?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }

  return (
    <header className="chat-tabs-bar">
      <h1 className="visually-hidden">{t("Chat")}</h1>
      {activeId && <PersonaSwitcher sessionID={activeId} disabled={personaDisabled} onChanged={onPersonaChanged} />}
      {assistantMood && (
        <div className="chat-mood-readout" role="status" aria-label={t("Assistant mood: {mood}", { mood: translateKnown(assistantMood) })}>
          <span>{t("Mood")}</span>
          <strong>{translateKnown(assistantMood)}</strong>
        </div>
      )}
      <div className="chat-tabs-track">
        <div
          ref={scrollRef}
          className="chat-tabs-scroll"
          style={{ width: preferredTabStripWidth(sessions.length) }}
        >
          <div className="chat-tabs-list" role="tablist" aria-label={t("Chat sessions")}>
            {sessions.map((session) => {
              const hasMenuActions = !session.saved || !session.active;
              return (
                <div
                  key={session.id}
                  ref={session.id === activeId ? activeRef : undefined}
                  className="chat-tab-wrap"
                  data-active={session.id === activeId || undefined}
                >
                  <button
                    type="button"
                    className="chat-tab"
                    id={`chat-tab-${session.id}`}
                    role="tab"
                    aria-selected={session.id === activeId}
                    aria-controls="active-chat-panel"
                    tabIndex={session.id === activeId ? 0 : -1}
                    disabled={disabled}
                    title={session.saved ? session.title : t("{title} (not saved)", { title: session.title })}
                    onClick={() => onActivate(session)}
                    onKeyDown={(event) => {
                      if (["ArrowRight", "ArrowLeft", "Home", "End"].includes(event.key)) {
                        event.preventDefault();
                        moveTabFocus(session, event.key);
                      }
                    }}
                    onContextMenu={(event) => {
                      event.preventDefault();
                      openMenu(session, event.clientX, event.clientY, event.currentTarget);
                    }}
                  >
                    <span>{session.title}</span>
                    {!session.saved && <span className="chat-tab-unsaved" aria-label={t("Not saved")} />}
                  </button>
                  {hasMenuActions && (
                    <button
                      type="button"
                      className="chat-tab-menu-button"
                      aria-label={t("Open options for {title}", { title: session.title })}
                      aria-haspopup="menu"
                      aria-expanded={target?.session.id === session.id}
                      aria-controls={target?.session.id === session.id ? menu.id : undefined}
                      disabled={disabled}
                      onClick={(event) => {
                        if (target?.session.id === session.id) menu.close();
                        else openFromButton(session, event.currentTarget);
                      }}
                      onKeyDown={(event) => menuButtonKeyDown(session, event)}
                    >
                      <MoreHorizontalIcon size={16} />
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </div>
        <div className="chat-new-slot">
          <button
            type="button"
            className="chat-new-button"
            aria-label={t("Start a new chat")}
            title={t("New chat")}
            disabled={disabled}
            onClick={onNew}
          >
            <PlusIcon />
          </button>
        </div>
      </div>
      {target && (
        <div
          {...menu.menuProps}
          className="chat-tab-menu"
          aria-label={t("{title} options", { title: target.session.title })}
          style={{ left: target.left, top: target.top }}
        >
          <button
            type="button"
            role="menuitem"
            disabled={target.session.saved}
            onClick={() => chooseAction(target.session.id, () => onSave(target.session))}
          >
            <SaveIcon size={16} />
            {target.session.saved ? t("Saved") : t("Save chat")}
          </button>
          <button
            type="button"
            role="menuitem"
            disabled={target.session.active}
            onClick={() => chooseAction(activeId, () => onDelete(target.session))}
          >
            <TrashIcon size={16} />{t("Delete chat")}</button>
        </div>
      )}
    </header>
  );
}

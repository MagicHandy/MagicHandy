import { useEffect, useRef, useState } from "react";
import type { ChatSession } from "../api/types";
import { MoreHorizontalIcon, PlusIcon, SaveIcon, TrashIcon } from "../shell/icons";

interface Props {
  sessions: ChatSession[];
  activeId: string;
  disabled: boolean;
  onActivate: (session: ChatSession) => void;
  onNew: () => void;
  onSave: (session: ChatSession) => void;
  onDelete: (session: ChatSession) => void;
}

interface MenuState {
  session: ChatSession;
  left: number;
  top: number;
  opener: HTMLElement;
}

export function ChatTabs({ sessions, activeId, disabled, onActivate, onNew, onSave, onDelete }: Props) {
  const [menu, setMenu] = useState<MenuState | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const activeRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    activeRef.current?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [activeId]);

  useEffect(() => {
    if (!menu) return;
    menuRef.current?.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenu(null);
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        menu.opener.focus();
        setMenu(null);
      }
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", key);
    };
  }, [menu]);

  function openMenu(session: ChatSession, left: number, top: number, opener: HTMLElement) {
    if (session.saved && session.active) {
      setMenu(null);
      return;
    }
    const menuWidth = 190;
    const menuHeight = 96;
    setMenu({
      session,
      left: Math.max(8, Math.min(left, window.innerWidth - menuWidth - 8)),
      top: Math.max(8, Math.min(top, window.innerHeight - menuHeight - 8)),
      opener,
    });
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
    document.getElementById(`chat-tab-${sessions[next].id}`)?.focus();
  }

  return (
    <header className="chat-tabs-bar">
      <h1 className="chat-tabs-title">Chat</h1>
      <div className="chat-tabs-track">
        <div className="chat-tabs-scroll">
          <div className="chat-tabs-list" role="tablist" aria-label="Chat sessions">
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
                    title={session.saved ? session.title : `${session.title} (not saved)`}
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
                    {!session.saved && <span className="chat-tab-unsaved" aria-label="Not saved" />}
                  </button>
                  {hasMenuActions && (
                    <button
                      type="button"
                      className="chat-tab-menu-button"
                      aria-label={`Open options for ${session.title}`}
                      aria-haspopup="menu"
                      aria-expanded={menu?.session.id === session.id}
                      disabled={disabled}
                      onClick={(event) => {
                        const rect = event.currentTarget.getBoundingClientRect();
                        openMenu(session, rect.right - 190, rect.bottom + 4, event.currentTarget);
                      }}
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
            className="icon-button chat-new-button"
            aria-label="Start a new chat"
            title="New chat"
            disabled={disabled}
            onClick={onNew}
          >
            <PlusIcon />
          </button>
        </div>
      </div>
      {menu && (
        <div
          ref={menuRef}
          className="chat-tab-menu"
          role="menu"
          aria-label={`${menu.session.title} options`}
          style={{ left: menu.left, top: menu.top }}
        >
          <button
            type="button"
            role="menuitem"
            disabled={menu.session.saved}
            onClick={() => { menu.opener.focus(); setMenu(null); onSave(menu.session); }}
          >
            <SaveIcon size={16} />
            {menu.session.saved ? "Saved" : "Save chat"}
          </button>
          <button
            type="button"
            role="menuitem"
            disabled={menu.session.active}
            onClick={() => { menu.opener.focus(); setMenu(null); onDelete(menu.session); }}
          >
            <TrashIcon size={16} />
            Delete chat
          </button>
        </div>
      )}
    </header>
  );
}

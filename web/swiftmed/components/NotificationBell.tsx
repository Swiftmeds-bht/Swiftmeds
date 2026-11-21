"use client";

import Link from "next/link";
import { useState, useTransition } from "react";

import { markNotificationsSeenAction } from "@/app/actions";
import { formatRelativeTime } from "@/lib/format";
import type { NotificationView } from "@/lib/notifications";

import { BellIcon } from "./Icons";
import { usePopover } from "./usePopover";

export interface NotificationBellProps {
  items: NotificationView[];
  unread: number;
  /** Creation time of the newest item, remembered as seen when the bell opens. */
  newest?: string;
  /** Set when the list could not be loaded. */
  failed?: boolean;
  /** For tests: what to call instead of the server action. */
  onSeen?: (newest: string) => Promise<void>;
}

const countLabel = (n: number) => (n > 9 ? "9+" : String(n));

/** The header bell (SCRUM-137): hold updates and back-in-stock alerts, newest first. */
export function NotificationBell({ items, unread, newest, failed, onSeen = markNotificationsSeenAction }: NotificationBellProps) {
  const [shownUnread, setShownUnread] = useState(unread);
  const [, startTransition] = useTransition();
  const { open, close, toggle, rootRef, buttonRef, panelId } = usePopover(() => {
    if (shownUnread > 0 && newest) {
      setShownUnread(0);
      startTransition(() => onSeen(newest));
    }
  });

  const label = shownUnread > 0 ? `Notifications, ${shownUnread} unread` : "Notifications";

  return (
    <div className="popover" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="icon-button"
        aria-label={label}
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
      >
        <BellIcon size={20} />
        {shownUnread > 0 ? (
          <span className="bell-count" aria-hidden="true">
            {countLabel(shownUnread)}
          </span>
        ) : null}
      </button>
      <div id={panelId} hidden={!open} className="popover-panel bell-panel" role="region" aria-label="Notifications">
        <p className="popover-title">Notifications</p>
        {failed ? (
          <p className="popover-empty">Notifications could not be loaded. Reload the page to try again.</p>
        ) : items.length === 0 ? (
          <p className="popover-empty">Nothing yet. Hold updates and back-in-stock alerts appear here.</p>
        ) : (
          <ul className="bell-list">
            {items.map((n) => {
              const content = (
                <>
                  <span className="bell-item-title">{n.title}</span>
                  <span className="bell-item-body">{n.body}</span>
                  {n.createdAt ? (
                    // Server and browser can round "minutes ago" differently.
                    <time className="bell-item-time" dateTime={n.createdAt} suppressHydrationWarning>
                      {formatRelativeTime(n.createdAt)}
                    </time>
                  ) : null}
                </>
              );
              return (
                <li key={n.id}>
                  {n.href ? (
                    <Link href={n.href} className="bell-item" onClick={() => close(false)}>
                      {content}
                    </Link>
                  ) : (
                    <div className="bell-item">{content}</div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

"use client";

import Link from "next/link";

import { BellIcon } from "./Icons";
import { usePopover } from "./usePopover";

export interface WaitingHold {
  holdId: string;
  medicine: string;
  quantity: number;
  /** "1 h 20 min left" */
  timeLeft: string;
}

const countLabel = (n: number) => (n > 9 ? "9+" : String(n));

/**
 * The staff bell (SCRUM-138): holds waiting for pickup at this pharmacy. Every
 * one needs action at the counter, so the count is simply how many there are.
 */
export function HoldsBell({ holds, failed }: { holds: WaitingHold[]; failed?: boolean }) {
  const { open, close, toggle, rootRef, buttonRef, panelId } = usePopover();
  const count = holds.length;
  const label = count > 0 ? `Holds waiting, ${count}` : "Holds waiting";

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
        {count > 0 ? (
          <span className="bell-count" aria-hidden="true">
            {countLabel(count)}
          </span>
        ) : null}
      </button>
      <div id={panelId} hidden={!open} className="popover-panel bell-panel" role="region" aria-label="Holds waiting">
        <p className="popover-title">Waiting for pickup</p>
        {failed ? (
          <p className="popover-empty">Holds could not be loaded. Reload the page to try again.</p>
        ) : count === 0 ? (
          <p className="popover-empty">No one is waiting. New holds appear here.</p>
        ) : (
          <>
            <ul className="bell-list">
              {holds.map((h) => (
                <li key={h.holdId}>
                  <Link href={`/holds#hold-${h.holdId}`} className="bell-item" onClick={() => close(false)}>
                    <span className="bell-item-title">{h.medicine}</span>
                    <span className="bell-item-body">
                      {h.quantity} {h.quantity === 1 ? "unit" : "units"}
                      {h.timeLeft ? ` · ${h.timeLeft}` : ""}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
            <Link href="/holds" className="popover-footer-link" onClick={() => close(false)}>
              Collect holds
            </Link>
          </>
        )}
      </div>
    </div>
  );
}

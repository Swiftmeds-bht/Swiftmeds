"use client";

import { logoutAction } from "@/app/actions";

import { ChevronDownIcon } from "./Icons";
import { usePopover } from "./usePopover";

export interface AccountMenuProps {
  /** The pharmacy this account works at. */
  pharmacy: string;
  /** Saved at sign-in; older sessions may not have it. */
  email?: string;
}

/** The account button (SCRUM-138): who is signed in, for which pharmacy, and Sign out. */
export function AccountMenu({ pharmacy, email }: AccountMenuProps) {
  const { open, toggle, rootRef, buttonRef, panelId } = usePopover();
  const initial = ([...(email?.trim() || pharmacy)][0] ?? "?").toLocaleUpperCase();

  return (
    <div className="popover" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="account-button"
        aria-label={`Account: ${email ?? pharmacy}`}
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
      >
        <span className="avatar" aria-hidden="true">
          {initial}
        </span>
        <ChevronDownIcon size={16} />
      </button>
      <div id={panelId} hidden={!open} className="popover-panel account-panel" role="region" aria-label="Account">
        <div className="account-who">
          <p className="account-name">{pharmacy}</p>
          <p className="account-email">{email ? `Pharmacy staff · ${email}` : "Pharmacy staff"}</p>
        </div>
        <form action={logoutAction} className="account-signout">
          <button type="submit" className="link-button">
            Sign out
          </button>
        </form>
      </div>
    </div>
  );
}

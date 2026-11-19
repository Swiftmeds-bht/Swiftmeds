"use client";

import Link from "next/link";

import { logoutAction } from "@/app/actions";

import { ChevronDownIcon } from "./Icons";
import { usePopover } from "./usePopover";

export interface AccountMenuProps {
  email: string;
  /** The saved display name, when the patient has one. */
  name?: string;
  initial: string;
  /** For pharmacy staff: the pharmacy they work at (SCRUM-139). */
  pharmacy?: string;
}

/** The account button (SCRUM-137): who is signed in, the profile, and signing out. */
export function AccountMenu({ email, name, initial, pharmacy }: AccountMenuProps) {
  const { open, close, toggle, rootRef, buttonRef, panelId } = usePopover();
  return (
    <div className="popover" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="account-button"
        aria-label={`Account: ${name ?? email}`}
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
          {name ? <p className="account-name">{name}</p> : null}
          <p className="account-email">{email}</p>
          {pharmacy ? <p className="account-email">Pharmacy staff · {pharmacy}</p> : null}
        </div>
        <ul className="account-links">
          <li>
            <Link href="/profile" className="account-link" onClick={() => close(false)}>
              {name ? "Your profile" : "Add your name and phone"}
            </Link>
          </li>
        </ul>
        <form action={logoutAction} className="account-signout">
          <button type="submit" className="link-button">
            Sign out
          </button>
        </form>
      </div>
    </div>
  );
}

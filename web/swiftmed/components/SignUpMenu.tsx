"use client";

import Link from "next/link";

import { PATIENT_SIGNUP, PHARMACY_SIGNUP } from "@/lib/roles";

import { ChevronDownIcon } from "./Icons";
import { usePopover } from "./usePopover";

/**
 * The signed-out "Sign up" button (SCRUM-142), the twin of the Log in menu:
 * one entry for patients and one for pharmacies, which register and wait for
 * approval.
 */
export function SignUpMenu() {
  const { open, close, toggle, rootRef, buttonRef, panelId } = usePopover();
  return (
    <div className="popover" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="button button-small header-cta signup-button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
      >
        Sign up
        <ChevronDownIcon size={16} />
      </button>
      <div id={panelId} hidden={!open} className="popover-panel login-panel" role="region" aria-label="Sign up">
        <section className="login-group" aria-labelledby={`${panelId}-patients`}>
          <h2 id={`${panelId}-patients`} className="login-group-title">
            Patients
          </h2>
          <Link href={PATIENT_SIGNUP} className="account-link" onClick={() => close(false)}>
            Create an account
          </Link>
        </section>
        <section className="login-group" aria-labelledby={`${panelId}-pharmacies`}>
          <h2 id={`${panelId}-pharmacies`} className="login-group-title">
            Pharmacies
          </h2>
          <Link href={PHARMACY_SIGNUP} className="account-link" onClick={() => close(false)}>
            Register your pharmacy
          </Link>
        </section>
      </div>
    </div>
  );
}

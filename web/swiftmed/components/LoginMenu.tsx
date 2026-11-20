"use client";

import Link from "next/link";

import { PATIENT_LOGIN, STAFF_LOGIN } from "@/lib/roles";

import { ChevronDownIcon } from "./Icons";
import { usePopover } from "./usePopover";

/**
 * The signed-out "Log in" button (SCRUM-140). It opens a small menu with one
 * entry for patients and one for pharmacy staff, as booking sites commonly do,
 * so staff do not have to find their way in through "For pharmacies".
 */
export function LoginMenu() {
  const { open, close, toggle, rootRef, buttonRef, panelId } = usePopover();
  return (
    <div className="popover" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="login-button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
      >
        Log in
        <ChevronDownIcon size={16} />
      </button>
      <div id={panelId} hidden={!open} className="popover-panel login-panel" role="region" aria-label="Log in">
        <section className="login-group" aria-labelledby={`${panelId}-patients`}>
          <h2 id={`${panelId}-patients`} className="login-group-title">
            Patients
          </h2>
          <Link href={PATIENT_LOGIN} className="account-link" aria-label="Log in as a patient" onClick={() => close(false)}>
            Log in
          </Link>
        </section>
        <section className="login-group" aria-labelledby={`${panelId}-staff`}>
          <h2 id={`${panelId}-staff`} className="login-group-title">
            Pharmacy staff
          </h2>
          <Link href={STAFF_LOGIN} className="account-link" aria-label="Log in as pharmacy staff" onClick={() => close(false)}>
            Log in
          </Link>
        </section>
      </div>
    </div>
  );
}

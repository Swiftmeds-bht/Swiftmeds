"use client";

import { useEffect, useId, useRef, useState } from "react";

const OPENED = "swiftmed:popover";

/**
 * A button that opens a panel below it (the bell and the account menu). The
 * same behaviour as the patient app's: Escape or a click outside closes it,
 * Escape puts focus back on the button, and only one is open at a time.
 */
export function usePopover() {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelId = useId();

  function close(returnFocus: boolean) {
    setOpen(false);
    if (returnFocus) buttonRef.current?.focus();
  }

  function toggle() {
    if (open) {
      setOpen(false);
      return;
    }
    window.dispatchEvent(new CustomEvent(OPENED, { detail: panelId }));
    setOpen(true);
  }

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        buttonRef.current?.focus();
      }
    };
    const onPointer = (e: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onOther = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== panelId) setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onPointer);
    window.addEventListener(OPENED, onOther);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointer);
      window.removeEventListener(OPENED, onOther);
    };
  }, [open, panelId]);

  return { open, close, toggle, rootRef, buttonRef, panelId };
}

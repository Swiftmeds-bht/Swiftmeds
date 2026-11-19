"use client";

import { useEffect, useId, useState, type KeyboardEvent } from "react";

import { MAX_QUERY_LENGTH, MIN_QUERY_LENGTH, suggestionQuery } from "@/lib/rules";
import type { Suggestion } from "@/lib/types";

import { SearchIcon } from "./Icons";

const DEBOUNCE_MS = 250;

interface Props {
  defaultValue?: string;
}

// The medicine box with name suggestions (the ARIA "combobox with listbox popup" pattern).
// Without JavaScript it is a plain text box in a GET form.
export function MedicineCombobox({ defaultValue = "" }: Props) {
  const id = useId();
  const inputId = `${id}-q`;
  const listId = `${id}-list`;
  const hintId = `${id}-hint`;

  const [value, setValue] = useState(defaultValue);
  // The text the suggestions were asked for. It only changes when the patient types.
  const [typed, setTyped] = useState("");
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  useEffect(() => {
    const text = typed.trim();
    if (text.length < MIN_QUERY_LENGTH) return;

    const controller = new AbortController();
    const timer = setTimeout(async () => {
      try {
        const res = await fetch(`/api/suggest?prefix=${encodeURIComponent(text)}`, { signal: controller.signal });
        if (!res.ok) throw new Error(`suggestions: HTTP ${res.status}`);
        const body = (await res.json()) as { suggestions?: Suggestion[] };
        setSuggestions(body.suggestions ?? []);
        setActive(-1);
        setOpen(true);
      } catch {
        // Suggestions are optional: if they fail, the patient just types the whole name.
        if (!controller.signal.aborted) setSuggestions([]);
      }
    }, DEBOUNCE_MS);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [typed]);

  const visible = open && typed.trim().length >= MIN_QUERY_LENGTH ? suggestions : [];
  const showList = visible.length > 0;

  function choose(suggestion: Suggestion) {
    setValue(suggestionQuery(suggestion));
    setOpen(false);
    setActive(-1);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    switch (event.key) {
      case "ArrowDown":
        if (suggestions.length === 0) return;
        event.preventDefault();
        setOpen(true);
        setActive((i) => (i + 1) % suggestions.length);
        break;
      case "ArrowUp":
        if (suggestions.length === 0) return;
        event.preventDefault();
        setOpen(true);
        setActive((i) => (i <= 0 ? suggestions.length - 1 : i - 1));
        break;
      case "Enter":
        // Enter picks the highlighted suggestion; with none highlighted it submits the form.
        if (showList && active >= 0) {
          event.preventDefault();
          choose(visible[active]);
        }
        break;
      case "Escape":
        if (showList) {
          event.preventDefault();
          setOpen(false);
          setActive(-1);
        }
        break;
    }
  }

  return (
    <div className="field combobox">
      <label htmlFor={inputId}>Medicine name</label>
      <div className="input-wrap">
        <SearchIcon size={20} />
        <input
        id={inputId}
        name="q"
        type="text"
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={showList}
        aria-controls={listId}
        aria-activedescendant={showList && active >= 0 ? `${listId}-${active}` : undefined}
        aria-describedby={hintId}
        required
        minLength={MIN_QUERY_LENGTH}
        maxLength={MAX_QUERY_LENGTH}
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setTyped(event.target.value);
          setOpen(true);
        }}
        onKeyDown={onKeyDown}
        onBlur={() => setOpen(false)}
        placeholder="e.g. Paracetamol"
        autoComplete="off"
        autoCapitalize="off"
        spellCheck={false}
        enterKeyHint="search"
        />
      </div>
      <p id={hintId} className="hint">
        Brand or generic name.
      </p>
      {showList && (
        <ul id={listId} role="listbox" aria-label="Suggested medicines" className="suggestions">
          {visible.map((s, i) => (
            <li
              key={s.productId}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              // mousedown, not click: the input must keep focus, or the list would close before the click lands.
              onMouseDown={(event) => {
                event.preventDefault();
                choose(s);
              }}
            >
              {s.displayName}
            </li>
          ))}
        </ul>
      )}
      <p role="status" className="sr-only">
        {showList ? `${visible.length} ${visible.length === 1 ? "suggestion" : "suggestions"} available. Use the up and down arrow keys to review.` : ""}
      </p>
    </div>
  );
}

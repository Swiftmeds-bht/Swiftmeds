import type { Suggestion } from "./types";

// The Search service accepts 2..100 characters after trimming (services/search/README.md).
export const MIN_QUERY_LENGTH = 2;
export const MAX_QUERY_LENGTH = 100;

/**
 * The text to search for after the patient picks a suggestion:
 * "Napa 500mg (Paracetamol)" becomes "Napa 500mg".
 */
export function suggestionQuery(suggestion: Suggestion): string {
  const name = suggestion.displayName;
  const bracket = name.indexOf(" (");
  return (bracket > 0 ? name.slice(0, bracket) : name).trim();
}

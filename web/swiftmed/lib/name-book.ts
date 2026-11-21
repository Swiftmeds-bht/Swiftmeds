// The names a page has looked up (SCRUM-138). No imports, so client components
// that format holds can use it without pulling the lookups into the browser.
export interface NameBook {
  products: ReadonlyMap<string, { brandName: string; strength?: string }>;
  pharmacies: ReadonlyMap<string, { name: string; address?: string }>;
}

export const EMPTY_BOOK: NameBook = { products: new Map(), pharmacies: new Map() };

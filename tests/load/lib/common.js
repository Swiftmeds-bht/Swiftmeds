// Shared helpers for the k6 load tests (SCRUM-77).
import http from "k6/http";

export const BASE_URL = (__ENV.BASE_URL || "http://localhost:8080").replace(/\/+$/, "");

// The demo seed (deploy/seed). Pharmacy 01 has exactly one pack of product 02:
// the "last unit" the reservation burst fights over.
export const SEED = {
  pharmacyId: "40000000-0000-4000-8000-000000000001",
  lastUnitProductId: "10000000-0000-4000-8000-000000000002",
  lastUnitQuery: "Paracetamol Syrup",
  // Thimphu, next to pharmacy 01.
  lat: 27.4728,
  lng: 89.639,
};

export function jsonHeaders(token) {
  const headers = { "Content-Type": "application/json", Accept: "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

export function search(q, lat, lng, tags = {}) {
  const query = `q=${encodeURIComponent(q)}&lat=${lat}&lng=${lng}`;
  return http.get(`${BASE_URL}/api/v1/search?${query}`, {
    headers: { Accept: "application/json" },
    tags: { name: "search", ...tags },
  });
}

/** Units available for one product at one pharmacy, as patients see it in search. */
export function availableInSearch(q, productId, pharmacyId, lat, lng) {
  const res = search(q, lat, lng, { name: "search-verify" });
  if (res.status !== 200) throw new Error(`search for "${q}" failed: HTTP ${res.status}`);
  const product = (res.json("results") || []).find((r) => r.productId === productId);
  const offer = product && (product.offers || []).find((o) => o.pharmacyId === pharmacyId);
  return offer ? offer.quantityAvailable || 0 : 0;
}

/** Writes the end-of-test summary as JSON and as one Markdown block for the test summary. */
export function writeSummary(name, data, extraLines = []) {
  const metric = (key, stat) => {
    const m = data.metrics[key];
    return m && m.values && m.values[stat] !== undefined ? m.values[stat] : undefined;
  };
  const ms = (v) => (v === undefined ? "n/a" : `${Math.round(v)} ms`);
  const pct = (v) => (v === undefined ? "n/a" : `${(v * 100).toFixed(2)}%`);
  const thresholds = Object.entries(data.metrics)
    .filter(([, m]) => m.thresholds)
    .map(([key, m]) =>
      Object.entries(m.thresholds).map(([expr, t]) => `| \`${key}\` | \`${expr}\` | ${t.ok ? "pass" : "**FAIL**"} |`),
    )
    .flat();

  const md = [
    `### ${name} (${new Date().toISOString()})`,
    "",
    `Target: \`${BASE_URL}\``,
    "",
    `- Requests: ${metric("http_reqs", "count") ?? 0}, failed: ${pct(metric("http_req_failed", "rate"))}`,
    `- Latency: p50 ${ms(metric("http_req_duration", "med"))}, p95 ${ms(metric("http_req_duration", "p(95)"))}, max ${ms(metric("http_req_duration", "max"))}`,
    ...extraLines,
    "",
    "| Metric | Threshold | Result |",
    "|---|---|---|",
    ...thresholds,
    "",
  ].join("\n");

  return {
    stdout: `\n${md}\n`,
    [`results/${name}.json`]: JSON.stringify(data, null, 2),
    [`results/${name}.md`]: md,
  };
}

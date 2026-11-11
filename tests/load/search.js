// k6 load test: medicine search at 100 concurrent users (SCRUM-77, TC-035,
// SRS-NFR-03). Search is public, so no accounts are needed.
//
//   cd tests/load && k6 run search.js
//   BASE_URL=http://localhost:8080 VUS=100 HOLD=2m k6 run search.js
//
// Pass: p95 of search below 2 s and fewer than 1% failed requests.
import { check, sleep } from "k6";
import { SharedArray } from "k6/data";
import { Trend } from "k6/metrics";

import { search, SEED, writeSummary } from "./lib/common.js";

const VUS = Number(__ENV.VUS || 100);
const HOLD = __ENV.HOLD || "2m";

// Brand and generic names from the catalog seed, so most searches find stock.
const QUERIES = new SharedArray("queries", () => {
  const lines = open("../../deploy/seed/medicines.csv").trim().split(/\r?\n/).slice(1);
  const names = new Set();
  for (const line of lines) {
    const [, brand, generic] = line.split(",");
    if (brand) names.add(brand.trim());
    if (generic) names.add(generic.trim());
  }
  return [...names];
});

// Patients spread around Thimphu, Paro and Chukha (the seeded dzongkhags).
const ORIGINS = [
  [SEED.lat, SEED.lng],
  [27.4305, 89.4133],
  [26.8526, 89.3884],
];

const degraded = new Trend("search_degraded", false);

export const options = {
  scenarios: {
    search: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: VUS },
        { duration: HOLD, target: VUS },
        { duration: "10s", target: 0 },
      ],
      gracefulRampDown: "10s",
    },
  },
  thresholds: {
    "http_req_duration{name:search}": ["p(95)<2000"],
    "http_req_failed{name:search}": ["rate<0.01"],
    checks: ["rate>0.99"],
  },
};

export default function () {
  const q = QUERIES[Math.floor(Math.random() * QUERIES.length)];
  const [lat, lng] = ORIGINS[Math.floor(Math.random() * ORIGINS.length)];
  // Up to ~1 km of jitter so requests do not all hit the same cache entry.
  const res = search(q, lat + (Math.random() - 0.5) * 0.02, lng + (Math.random() - 0.5) * 0.02);

  check(res, {
    "search answers 200": (r) => r.status === 200,
    "search returns a results list": (r) => r.status !== 200 || Array.isArray(r.json("results")),
  });
  degraded.add(res.headers["X-Search-Degraded"] ? 1 : 0);

  // A person reads results before searching again.
  sleep(1 + Math.random() * 2);
}

export function handleSummary(data) {
  const share = data.metrics.search_degraded && data.metrics.search_degraded.values.avg;
  return writeSummary("search", data, [
    `- Virtual users: ${VUS} (hold ${HOLD})`,
    `- Degraded answers (optional service down): ${share === undefined ? "n/a" : `${(share * 100).toFixed(2)}%`}`,
  ]);
}

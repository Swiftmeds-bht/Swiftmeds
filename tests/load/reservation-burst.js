// k6 load test: many patients reserve the same last unit at once (SCRUM-77,
// TC-036, SRS-NFR-03). The hold saga must never let more holds become ACTIVE
// than there were units available.
//
//   cd tests/load && k6 run reservation-burst.js
//   BASE_URL=http://localhost:8080 BURST_VUS=50 k6 run reservation-burst.js
//
// Needs the gateway auth and holds routes (SCRUM-115, SCRUM-54). Each virtual
// user is a separate patient, because a patient may have only a few open holds.
// Patients are registered in setup(), or pass PATIENT_TOKENS (comma-separated
// access tokens) to reuse existing ones.
//
// Optional:
//   STAFF_TOKEN   access token of a staff member of the seeded pharmacy 01. When set,
//                 the stock is reset to exactly one available unit before the burst,
//                 and quantityReserved is checked against quantityOnHand afterwards.
//   CLEANUP=false keep the winning hold instead of cancelling it in teardown.
//   PHARMACY_ID, PRODUCT_ID, PRODUCT_QUERY  burst on another item.
//
// Pass: oversold_units == 0, every hold resolves, and create-hold p95 < 2 s.
import exec from "k6/execution";
import http from "k6/http";
import { check, fail, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";

import { availableInSearch, BASE_URL, jsonHeaders, SEED, writeSummary } from "./lib/common.js";

const BURST_VUS = Number(__ENV.BURST_VUS || 50);
const PHARMACY_ID = __ENV.PHARMACY_ID || SEED.pharmacyId;
const PRODUCT_ID = __ENV.PRODUCT_ID || SEED.lastUnitProductId;
const PRODUCT_QUERY = __ENV.PRODUCT_QUERY || SEED.lastUnitQuery;
const RESOLVE_TIMEOUT_S = Number(__ENV.RESOLVE_TIMEOUT_S || 15);

const holdsActive = new Counter("holds_active");
const holdsRejected = new Counter("holds_rejected");
const holdsUnresolved = new Counter("holds_unresolved");
const holdsRefused = new Counter("holds_refused");
const oversoldUnits = new Counter("oversold_units");
const timeToResolve = new Trend("hold_time_to_resolve", true);

export const options = {
  setupTimeout: "5m",
  scenarios: {
    burst: {
      // Every patient fires exactly one reservation at the same moment.
      executor: "per-vu-iterations",
      vus: BURST_VUS,
      iterations: 1,
      maxDuration: "2m",
    },
  },
  thresholds: {
    oversold_units: ["count==0"],
    holds_unresolved: ["count==0"],
    "http_req_duration{name:create-hold}": ["p(95)<2000"],
    "http_req_failed{name:create-hold}": ["rate<0.01"],
  },
};

function registerPatients(count) {
  const run = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
  const password = "k6-load-test-password";
  const tokens = [];
  for (let i = 0; i < count; i++) {
    const email = `k6-${run}-${i}@load.swiftmed.test`;
    const reg = http.post(
      `${BASE_URL}/api/v1/auth/register`,
      JSON.stringify({ email, password, role: "ROLE_PATIENT" }),
      { headers: jsonHeaders(), tags: { name: "setup-register" } },
    );
    if (reg.status !== 201 && reg.status !== 200) fail(`register ${email}: HTTP ${reg.status} ${reg.body}`);
    const login = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({ email, password }), {
      headers: jsonHeaders(),
      tags: { name: "setup-login" },
    });
    if (login.status !== 200) fail(`login ${email}: HTTP ${login.status} ${login.body}`);
    tokens.push(login.json("tokens.accessToken"));
  }
  return tokens;
}

function stockAsStaff() {
  const res = http.get(
    `${BASE_URL}/api/v1/pharmacies/${PHARMACY_ID}/stock/${PRODUCT_ID}`,
    { headers: jsonHeaders(__ENV.STAFF_TOKEN), tags: { name: "staff-stock" } },
  );
  if (res.status !== 200) fail(`read stock as staff: HTTP ${res.status} ${res.body}`);
  return res.json();
}

export function setup() {
  // Give the threshold counters a sample so they are evaluated even when zero.
  oversoldUnits.add(0);
  holdsUnresolved.add(0);

  const tokens = __ENV.PATIENT_TOKENS ? __ENV.PATIENT_TOKENS.split(",").map((t) => t.trim()) : registerPatients(BURST_VUS);
  if (tokens.length < BURST_VUS) fail(`need ${BURST_VUS} patient tokens, got ${tokens.length}`);

  if (__ENV.STAFF_TOKEN) {
    // Make exactly one unit available, whatever earlier runs left reserved.
    const reserved = stockAsStaff().quantityReserved || 0;
    const res = http.put(
      `${BASE_URL}/api/v1/pharmacies/${PHARMACY_ID}/stock/${PRODUCT_ID}`,
      JSON.stringify({ quantityOnHand: reserved + 1 }),
      { headers: jsonHeaders(__ENV.STAFF_TOKEN), tags: { name: "staff-reset-stock" } },
    );
    if (res.status !== 200) fail(`reset stock: HTTP ${res.status} ${res.body}`);
  }

  const available = availableInSearch(PRODUCT_QUERY, PRODUCT_ID, PHARMACY_ID, SEED.lat, SEED.lng);
  if (available < 1) {
    fail(`no unit of ${PRODUCT_ID} is available at ${PHARMACY_ID} before the burst; reset the seed or pass STAFF_TOKEN`);
  }
  console.log(`burst: ${BURST_VUS} patients reserve ${PRODUCT_ID} at ${PHARMACY_ID}; ${available} unit(s) available`);
  return { tokens: tokens.slice(0, BURST_VUS), available };
}

export default function (data) {
  const token = data.tokens[exec.vu.idInTest - 1];
  const res = http.post(
    `${BASE_URL}/api/v1/holds`,
    JSON.stringify({ pharmacyId: PHARMACY_ID, productId: PRODUCT_ID, quantity: 1 }),
    { headers: jsonHeaders(token), tags: { name: "create-hold" } },
  );

  // 202: PENDING, the saga decides. A 409 (no stock) is also a correct answer.
  const accepted = check(res, {
    "create hold is accepted or refused cleanly": (r) => r.status === 202 || r.status === 201 || r.status === 409,
  });
  if (!accepted || res.status === 409) {
    holdsRefused.add(1);
    return;
  }

  const holdId = res.json("holdId");
  const started = Date.now();
  let status = res.json("status");
  while (status === "HOLD_STATUS_PENDING" && Date.now() - started < RESOLVE_TIMEOUT_S * 1000) {
    sleep(0.25);
    const poll = http.get(`${BASE_URL}/api/v1/holds/${holdId}`, {
      headers: jsonHeaders(token),
      tags: { name: "poll-hold" },
    });
    if (poll.status === 200) status = poll.json("status");
  }
  timeToResolve.add(Date.now() - started);

  if (status === "HOLD_STATUS_ACTIVE") holdsActive.add(1);
  else if (status === "HOLD_STATUS_REJECTED") holdsRejected.add(1);
  else holdsUnresolved.add(1);
}

export function teardown(data) {
  // Count ACTIVE holds by asking every patient for theirs: each sees only their own.
  const winners = [];
  for (const token of data.tokens) {
    const res = http.get(`${BASE_URL}/api/v1/holds?status=HOLD_STATUS_ACTIVE&pageSize=50`, {
      headers: jsonHeaders(token),
      tags: { name: "verify-holds" },
    });
    if (res.status !== 200) continue;
    for (const hold of res.json("holds") || []) {
      if (hold.pharmacyId === PHARMACY_ID && hold.productId === PRODUCT_ID) winners.push({ token, holdId: hold.holdId });
    }
  }

  const oversold = Math.max(0, winners.length - data.available);
  const availableAfter = availableInSearch(PRODUCT_QUERY, PRODUCT_ID, PHARMACY_ID, SEED.lat, SEED.lng);
  let reservedOverOnHand = 0;
  if (__ENV.STAFF_TOKEN) {
    const stock = stockAsStaff();
    reservedOverOnHand = Math.max(0, (stock.quantityReserved || 0) - (stock.quantityOnHand || 0));
  }
  oversoldUnits.add(oversold + reservedOverOnHand);

  console.log(
    `burst result: ${winners.length} ACTIVE hold(s) for ${data.available} unit(s); ` +
      `available in search afterwards: ${availableAfter}; oversold: ${oversold + reservedOverOnHand}`,
  );
  check(availableAfter, { "search never shows negative stock": (v) => v >= 0 });

  if (__ENV.CLEANUP !== "false") {
    for (const { token, holdId } of winners) {
      http.post(`${BASE_URL}/api/v1/holds/${holdId}/cancel`, JSON.stringify({ reason: "k6 load test cleanup" }), {
        headers: jsonHeaders(token),
        tags: { name: "cleanup-cancel" },
      });
    }
  }
}

export function handleSummary(data) {
  const count = (key) => (data.metrics[key] ? data.metrics[key].values.count : 0);
  return writeSummary("reservation-burst", data, [
    `- Patients: ${BURST_VUS}, all reserving ${PRODUCT_ID} at ${PHARMACY_ID}`,
    `- Holds ACTIVE: ${count("holds_active")}, REJECTED: ${count("holds_rejected")}, refused: ${count("holds_refused")}, unresolved: ${count("holds_unresolved")}`,
    `- Oversold units: **${count("oversold_units")}**`,
  ]);
}

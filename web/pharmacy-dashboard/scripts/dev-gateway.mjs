// A small stand-in for the gateway routes the pharmacy dashboard uses, so the
// dashboard can be tried before those routes exist on the real gateway
// (SCRUM-115 auth, SCRUM-54 holds, stock and stock-sync). Development only;
// state is kept in memory and lost on restart.
//
//   npm run dev:gateway                            # listens on http://localhost:8080
//   GATEWAY_URL=http://localhost:8080 npm run dev  # in another terminal
//
// Sign in with staff@demo.bt / password (patient@demo.bt / password is refused).
// The pickup code for every demo hold is 482913; any other code answers 403.
// Uploaded CSVs are validated like services/stock-sync: header
// product_id,quantity[,price_chhertum]; unknown products and bad quantities are
// rejected per row.
import { randomUUID } from "node:crypto";
import { createServer } from "node:http";

const PORT = Number(process.env.PORT ?? 8080);
const PHARMACY_ID = "ph-demo-1";
const PICKUP_CODE = "482913";

const b64 = (value) => Buffer.from(JSON.stringify(value)).toString("base64url");
const token = (claims) => `${b64({ alg: "none", typ: "JWT" })}.${b64(claims)}.dev`;

const now = () => new Date().toISOString();
const stock = new Map(
  [
    ["p-napa-500", 24, 2],
    ["p-brufen-400", 8, 0],
    ["p-amoxil-250", 0, 0],
  ].map(([productId, onHand, reserved]) => [productId, { productId, onHand, reserved, confirmedAt: now() }]),
);
const holds = new Map(
  ["h-demo-1", "h-demo-2"].map((holdId) => [
    holdId,
    {
      holdId,
      userId: "u-patient",
      pharmacyId: PHARMACY_ID,
      productId: "p-napa-500",
      quantity: 1,
      status: "HOLD_STATUS_ACTIVE",
      createdAt: now(),
      expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(),
    },
  ]),
);
const jobs = new Map();

const stockLevel = (s) => ({
  pharmacyId: PHARMACY_ID,
  productId: s.productId,
  quantityOnHand: s.onHand,
  quantityReserved: s.reserved,
  quantityAvailable: Math.max(0, s.onHand - s.reserved),
  confirmedAt: s.confirmedAt,
});

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return Buffer.concat(chunks);
}

function importCsv(text, filename) {
  const lines = text.replace(/^﻿/, "").split(/\r?\n/).filter((line) => line.trim() !== "");
  const header = (lines.shift() ?? "").split(",").map((h) => h.trim()).join(",");
  const job = { jobId: randomUUID(), pharmacyId: PHARMACY_ID, filename, createdAt: now(), rowErrors: [] };
  if (header !== "product_id,quantity" && header !== "product_id,quantity,price_chhertum") {
    return { error: "CSV header must be product_id,quantity or product_id,quantity,price_chhertum" };
  }
  if (lines.length === 0) return { error: "CSV file has no data rows" };

  const seen = new Set();
  let applied = 0;
  lines.forEach((line, i) => {
    const rowNumber = i + 2;
    const [productId = "", quantity = ""] = line.split(",").map((f) => f.trim());
    const reject = (message) => job.rowErrors.push({ rowNumber, productId, message });
    if (!productId) return reject("product_id is required");
    if (!/^-?\d+$/.test(quantity)) return reject("quantity must be a whole number");
    if (Number(quantity) < 0) return reject("quantity must not be negative");
    if (seen.has(productId)) return reject("duplicate product_id");
    seen.add(productId);
    const item = stock.get(productId);
    if (!item) return reject("unknown product");
    if (Number(quantity) < item.reserved) return reject("quantity is lower than the reserved quantity");
    item.onHand = Number(quantity);
    item.confirmedAt = now();
    applied++;
  });

  Object.assign(job, {
    rowsTotal: lines.length,
    rowsApplied: applied,
    rowsRejected: job.rowErrors.length,
    status: applied === 0 ? "SYNC_JOB_STATUS_FAILED" : job.rowErrors.length ? "SYNC_JOB_STATUS_PARTIAL" : "SYNC_JOB_STATUS_COMPLETED",
    finishedAt: now(),
  });
  jobs.set(job.jobId, job);
  return { job };
}

createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://${req.headers.host}`);
  const path = url.pathname;
  const send = (status, body) => {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(body));
  };
  console.log(`${req.method} ${path}`);

  if (path === "/healthz") return send(200, { status: "ok" });

  if (req.method === "POST" && path === "/api/v1/auth/login") {
    const { email, password } = JSON.parse((await readBody(req)).toString() || "{}");
    if (password !== "password" || !["staff@demo.bt", "patient@demo.bt"].includes(email)) {
      return send(401, { error: "invalid email or password" });
    }
    const staff = email === "staff@demo.bt";
    const role = staff ? "ROLE_PHARMACY_STAFF" : "ROLE_PATIENT";
    const userId = staff ? "u-staff" : "u-patient";
    return send(200, {
      userId,
      role,
      tokens: {
        accessToken: token({ sub: userId, role, ...(staff ? { pharmacy_id: PHARMACY_ID } : {}) }),
        refreshToken: randomUUID(),
        accessTokenExpiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
      },
    });
  }

  if (!req.headers.authorization?.startsWith("Bearer ")) return send(401, { error: "unauthenticated" });
  if (req.method === "GET" && path === "/api/v1/pharmacies/me/analytics/unmet-demand") {
    return send(200, {
      items: [
        {
          productId: "p-napa-500",
          productName: "Demo Paracetamol 500",
          searchCount: 19,
          unmetCount: 8,
        },
        {
          productId: "p-amoxil-250",
          productName: "Demo Amoxicillin 250",
          searchCount: 11,
          unmetCount: 4,
        },
      ],
    });
  }
  const base = `/api/v1/pharmacies/${PHARMACY_ID}`;
  if (req.method === "GET" && path === `${base}/stock`) {
    return send(200, { stock: [...stock.values()].map(stockLevel), page: {} });
  }

  const stockMatch = path.match(new RegExp(`^${base}/stock/([^/]+)$`));
  if (req.method === "PUT" && stockMatch) {
    const item = stock.get(decodeURIComponent(stockMatch[1]));
    if (!item) return send(404, { error: "stock not found" });
    const { quantityOnHand } = JSON.parse((await readBody(req)).toString() || "{}");
    if (!Number.isInteger(quantityOnHand) || quantityOnHand < 0) return send(400, { error: "quantity must not be negative" });
    if (quantityOnHand < item.reserved) return send(400, { error: "quantity is lower than the quantity reserved by active holds" });
    item.onHand = quantityOnHand;
    item.confirmedAt = now();
    return send(200, stockLevel(item));
  }

  if (req.method === "POST" && path === `${base}/stock-sync/jobs`) {
    const raw = await readBody(req);
    if (raw.length > (1 << 20) + 16_384) return send(413, { error: "file exceeds 1 MB" });
    // Good enough for a dev stand-in: take the first part's body from the multipart payload.
    const boundary = req.headers["content-type"]?.match(/boundary=(?:"([^"]+)"|([^;]+))/);
    if (!boundary) return send(400, { error: "request must be multipart/form-data" });
    const body = raw.toString("utf8");
    const part = body.split(`--${boundary[1] ?? boundary[2]}`)[1] ?? "";
    const filename = part.match(/filename="([^"]*)"/)?.[1] ?? "upload.csv";
    const content = part.slice(part.indexOf("\r\n\r\n") + 4).replace(/\r\n$/, "");
    const result = importCsv(content, filename);
    return result.error ? send(400, { error: result.error }) : send(201, result.job);
  }

  const jobMatch = path.match(new RegExp(`^${base}/stock-sync/jobs/([^/]+)$`));
  if (req.method === "GET" && jobMatch) {
    const job = jobs.get(decodeURIComponent(jobMatch[1]));
    return job ? send(200, job) : send(404, { error: "job not found" });
  }

  if (req.method === "GET" && path === "/api/v1/holds") {
    const status = url.searchParams.get("status");
    return send(200, { holds: [...holds.values()].filter((h) => !status || h.status === status), page: {} });
  }

  const collectMatch = path.match(/^\/api\/v1\/holds\/([^/]+)\/collect$/);
  if (req.method === "POST" && collectMatch) {
    const hold = holds.get(decodeURIComponent(collectMatch[1]));
    if (!hold) return send(404, { error: "hold not found" });
    const { pickupCode } = JSON.parse((await readBody(req)).toString() || "{}");
    if (hold.status !== "HOLD_STATUS_ACTIVE") return send(412, { error: "hold is not active" });
    if (String(pickupCode ?? "").replace(/\s/g, "") !== PICKUP_CODE) return send(403, { error: "wrong pickup code" });
    hold.status = "HOLD_STATUS_COLLECTED";
    hold.statusChangedAt = now();
    const item = stock.get(hold.productId);
    if (item) {
      item.onHand -= hold.quantity;
      item.reserved -= hold.quantity;
    }
    return send(200, hold);
  }

  send(404, { error: "not found" });
}).listen(PORT, () => console.log(`dev gateway on http://localhost:${PORT}`));

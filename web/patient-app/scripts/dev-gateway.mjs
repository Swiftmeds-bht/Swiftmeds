// A tiny stand-in for the gateway's search routes, so the patient app can be tried
// with real network calls before the real routes exist. Development only.
//
//   npm run dev:gateway                            # listens on http://localhost:8080
//   GATEWAY_URL=http://localhost:8080 npm run dev  # in another terminal
//
// What you type in the search box decides what happens:
//   "slow"     answers after 2.5 s (shows the loading state)
//   "hang"     never answers in time (timeout error)
//   "error"    HTTP 503 (error state)      "gone"  HTTP 404 (route not available)
//   "bad"      HTTP 400                    "degraded"  works, but reports optional services as down
//   "none"     no medicine matches         "nostock"   matches, but nobody has it; substitutes are shown
//   "napa", "paracetamol", "brufen", "ibuprofen"   normal results
import { createServer } from "node:http";

const PORT = Number(process.env.PORT ?? 8080);
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const offer = (n, pharmacyName, distanceM, quantityAvailable, priceChhertum, freshnessScore) => ({
  pharmacyId: `ph-${n}`,
  pharmacyName,
  address: `Norzin Lam ${n}, Thimphu`,
  distanceM,
  quantityAvailable,
  priceChhertum: String(priceChhertum), // protobuf JSON sends int64 as a string
  freshnessScore,
  trustScore: 0.8,
  rankScore: 1 - n / 10,
});
const product = (productId, brandName, genericName, strength, offers) => ({ productId, brandName, genericName, strength, offers });

const NAPA = product("p-napa", "Napa 500", "Paracetamol", "500mg", [
  offer(1, "Druk Pharmacy", 420, 24, 4500, 0.9),
  offer(2, "Norzin Chemist", 1300, 3, 5000, 0.5),
  offer(3, "Wangchuk Medical", 2600, 12, 0, 0.2),
]);
const BRUFEN = product("p-brufen", "Brufen 400", "Ibuprofen", "400mg", [offer(4, "Druk Pharmacy", 420, 8, 6000, 0.85)]);
const BRUFEN_OUT = product("p-brufen", "Brufen 400", "Ibuprofen", "400mg", []);
const IBUGESIC = product("p-ibugesic", "Ibugesic 400", "Ibuprofen", "400mg", [offer(5, "Norzin Chemist", 1300, 5, 5500, 0.6)]);
const NAMES = ["Napa 500mg (Paracetamol)", "Napa Extra (Paracetamol)", "Brufen 400 (Ibuprofen)", "Ibugesic 400 (Ibuprofen)"];

function search(q) {
  if (q.includes("none")) return { searchId: "dev", results: [], substitutes: [] };
  if (q.includes("nostock")) return { searchId: "dev", results: [BRUFEN_OUT], substitutes: [IBUGESIC] };
  const results = [];
  if (/napa|paracetamol/.test(q)) results.push(NAPA);
  if (/brufen|ibuprofen/.test(q)) results.push(BRUFEN);
  return { searchId: "dev", results, substitutes: [] };
}

createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://${req.headers.host}`);
  const send = (status, body, headers = {}) => {
    res.writeHead(status, { "Content-Type": "application/json", ...headers });
    res.end(JSON.stringify(body));
  };

  if (url.pathname === "/healthz") return send(200, { status: "ok" });

  if (url.pathname === "/api/v1/search") {
    const q = (url.searchParams.get("q") ?? "").toLowerCase();
    console.log(`search q=${q} lat=${url.searchParams.get("lat")} lng=${url.searchParams.get("lng")} radius_m=${url.searchParams.get("radius_m")}`);
    if (q.includes("error")) return send(503, { error: "search is unavailable" });
    if (q.includes("bad")) return send(400, { error: "invalid query" });
    if (q.includes("gone")) return send(404, { error: "not found" });
    if (q.includes("hang")) await sleep(30_000);
    if (q.includes("slow")) await sleep(2_500);
    return send(200, search(q), q.includes("degraded") ? { "x-search-degraded": "freshness,pricing" } : {});
  }

  if (url.pathname === "/api/v1/search/autocomplete") {
    const prefix = (url.searchParams.get("prefix") ?? "").toLowerCase();
    const suggestions = NAMES.filter((n) => n.toLowerCase().includes(prefix)).map((displayName, i) => ({ productId: `s-${i}`, displayName }));
    return send(200, { suggestions });
  }

  send(404, { error: "not found" });
}).listen(PORT, () => console.log(`dev gateway on http://localhost:${PORT}`));

// `npm run dev:demo`: the dev server on the built-in demo data (USE_DEMO_DATA=true),
// for working on screens without a gateway. Works the same on Windows and Linux.
// Extra arguments go to `next dev`, e.g. `npm run dev:demo -- -p 3071`.
import { spawn } from "node:child_process";

const child = spawn("next", ["dev", ...process.argv.slice(2)], {
  stdio: "inherit",
  shell: process.platform === "win32",
  env: { ...process.env, USE_DEMO_DATA: "true" },
});
child.on("exit", (code, signal) => process.exit(signal ? 1 : (code ?? 0)));

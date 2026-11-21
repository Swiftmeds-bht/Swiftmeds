// Forwards localhost ports to the Compose stack, for Cypress running in Docker.
//
// The web app sets its session cookie with the Secure flag in production.
// Browsers accept Secure cookies over plain http only from localhost, so the
// browser must see http://localhost:3000, not http://traefik:3000. Inside the
// Cypress container this forwards 127.0.0.1:<port> to <host>:<port>.
//
//   node scripts/forward.mjs traefik 3000 8080
import net from "node:net";

const [host, ...ports] = process.argv.slice(2);
if (!host || ports.length === 0) {
  console.error("usage: node forward.mjs <host> <port>...");
  process.exit(2);
}

for (const port of ports.map(Number)) {
  net
    .createServer((client) => {
      const upstream = net.connect(port, host);
      client.pipe(upstream).pipe(client);
      const close = () => {
        client.destroy();
        upstream.destroy();
      };
      client.on("error", close);
      upstream.on("error", close);
    })
    .listen(port, "127.0.0.1", () => console.log(`forwarding localhost:${port} -> ${host}:${port}`));
}

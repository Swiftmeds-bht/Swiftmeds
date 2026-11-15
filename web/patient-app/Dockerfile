# syntax=docker/dockerfile:1
# The patient app as a small Next.js standalone server (SCRUM-117).
# Build from THIS FOLDER (Compose does: deploy/docker-compose.yml, service patient-app):
#   docker build -t swiftmed/patient-app web/patient-app
# At run time set GATEWAY_URL (e.g. http://gateway:8080), or USE_DEMO_DATA=true.

# ---- dependencies: cached until package-lock.json changes ----
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# ---- build: next build with output "standalone" (next.config.ts) ----
FROM node:22-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# public/ is optional in Next.js; create it so the copy below always works.
RUN npm run build && mkdir -p public

# ---- run: only the traced server files, as the unprivileged node user ----
FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["node", "-e", "fetch('http://127.0.0.1:3000/').then((r) => process.exit(r.status < 500 ? 0 : 1), () => process.exit(1))"]
CMD ["node", "server.js"]

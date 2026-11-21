# Deployment runbook (Kubernetes)

SwiftMed is deployed on **Kubernetes (k3s)**; Docker Compose stays the local
development stack ([ADR 0005](adr/0005-docker-compose-main-stack-kubernetes-demo.md)).
The deployment runs on the **team laptop**, where k3s runs in Docker through
[k3d](https://k3d.io). The manifests are plain Kustomize, so the same overlay
deploys to any k3s or Kubernetes cluster (a cloud VM later: install k3s there
and run the same commands).

| What | Where |
|---|---|
| SwiftMed (patients) | http://localhost:9000 |
| Pharmacy portal | http://localhost:9000/pharmacy-portal (same app; staff sign in at `/login`) |
| API gateway | http://localhost:9080 (`/healthz`) |
| Public HTTPS URLs | Cloudflare quick tunnels, see [Public access](#public-access); `deploy/k8s/overlays/prod/tunnel-urls.sh` prints them. Fixed API address on ngrok for demos (SCRUM-156) |

**Demo accounts** (seeded by the seed job, password `swiftmeds-demo` for all):

| Account | Role | Use it for |
|---|---|---|
| `patient@demo.swiftmeds.test` | patient | search, reserve, holds, watchlist |
| `demo-tim-001@demo.swiftmeds.test` | pharmacist of Demo Pharmacy Thimphu 01 | the dashboard: stock, collecting holds, CSV upload |
| `demo-<licence>@demo.swiftmeds.test` | pharmacist of each other demo pharmacy (`deploy/seed/pharmacies.csv`) | the same, for that pharmacy |
| `admin@demo.swiftmeds.test` | admin | admin routes (and the pharmacy routes) |

All data is synthetic. New patients can also sign up in the app.

## Layout

```
deploy/k8s/
  base/                 every cluster: services, Traefik, Postgres, RabbitMQ, probes, limits,
                        HPAs, PodDisruptionBudgets, NetworkPolicies, nightly backup, seed
  overlays/prod/        the deployment: ghcr.io images at v1.0.0, 2 replicas of the busy
                        paths, bigger volumes, the Secret as a SealedSecret (committed, encrypted)
  overlays/local/       a development cluster: images built here (:dev), 1 replica,
                        the Secret from a local secrets.env (gitignored)
  platform/sealed-secrets/   the Sealed Secrets controller, once per cluster
  monitoring/           Prometheus, Grafana, Loki, Alloy, kube-state-metrics (namespace monitoring)
  tools/backup-shell.yaml    a helper pod for listing and restoring backups
```

CI checks that `base` and both overlays render and match the Kubernetes schemas
(`kustomize build | kubeconform -strict`, job "Kubernetes manifests"), so a
broken YAML file fails the PR. Pushing a `v*` tag builds the 23 images and pushes
them to `ghcr.io/swiftmeds-bht` (workflow "Release images").

## What runs

```
                 laptop :9000 :9080   (Cloudflare Tunnel, SCRUM-75)
                          |
              k3d load balancer -> Service traefik-public
                          |
                 Traefik x2 (web/patient entrypoints, grpc :50051)
               /                   \
        web (/ and              gateway x2+ --gRPC--> Traefik grpc :50051 --by :authority--> 19 gRPC services
   /pharmacy-portal)                                                       (search x2+, holds x2, inventory x2)
                                                                                    |
                                                         Postgres (StatefulSet) + nightly backup to its own volume
                                                         RabbitMQ (StatefulSet)
```

- **20 services + 2 web apps**, one Deployment, Service and Ingress route each.
- **Traefik inside the cluster**, configured like the Compose one. Services still
  call `traefik:50051` with the callee's name as `:authority`; Traefik sends each
  gRPC call to a pod directly, so replicas share the load (a plain Service would
  pin each HTTP/2 connection to one pod). k3s's bundled Traefik is switched off.
- **No single point of failure on the request path** (prod): Traefik, the gateway,
  Search, Holds and Inventory run 2 pods (the gateway and Search through their
  HPAs, minimum 2, up to 4 on CPU), each with a PodDisruptionBudget
  (`minAvailable: 1`). Postgres and RabbitMQ stay single (see ADR 0005).
- **Traefik is active-active** (SCRUM-135). Both replicas serve at the same time:
  each reads the Ingresses from the Kubernetes API and builds the same routes on
  its own (Traefik keeps no state), and the `traefik` and `traefik-public` Services
  spread connections over both. When one stops, its `preStop` pause and HTTP/2
  GOAWAY move clients to the other. Updates start the new pod before stopping an
  old one (`maxSurge: 1, maxUnavailable: 0`), the PodDisruptionBudget keeps one
  through a node drain, and a preferred anti-affinity puts them on different
  nodes when there are several. **Limit:** the laptop cluster has one node, so the
  node (and the laptop) is still a single point of failure. Surviving a node
  needs several nodes plus a load balancer in front of them (a cloud L4 load
  balancer, or MetalLB / keepalived for a floating IP): future work.
- **NetworkPolicies** (`base/networkpolicies.yaml`): everything is denied unless
  allowed. From outside, only Traefik's public ports (8080, 3000); inside,
  any pod may use Traefik's gRPC port; only Traefik reaches the services, the
  gateway and the web apps (and the web apps reach the gateway); only pods
  labelled `swiftmed/postgres-client` / `swiftmed/rabbitmq-client` reach Postgres
  and RabbitMQ.
- **Probes** (gRPC health, ready only after migrations ran), **requests and
  limits**, non-root pods with all capabilities dropped, and a `preStop` pause on
  every app container and on Traefik, so a pod keeps serving until it is out of
  the endpoints (zero failed calls on deletes and rolling updates, measured below).
- **Settings** in the ConfigMap `swiftmed-config` (`base/config.env`); **secrets**
  in `swiftmed-secrets` (keys: `base/secrets.env.example`), each pod getting only
  the keys it uses.

## Install (once per machine)

Needs Docker (with WSL 2 on Windows) and about 4 GB of free memory.

```bash
mkdir -p ~/.local/bin && cd ~/.local/bin
curl -fsSLo kubectl https://dl.k8s.io/release/v1.36.6/bin/linux/amd64/kubectl
curl -fsSLo k3d https://github.com/k3d-io/k3d/releases/download/v5.9.0/k3d-linux-amd64
curl -fsSL https://github.com/bitnami/sealed-secrets/releases/download/v0.41.2/kubeseal-0.41.2-linux-amd64.tar.gz | tar -xz kubeseal
chmod +x kubectl k3d kubeseal
```

## Deploy the production overlay

```bash
k3d cluster create --config deploy/k8s/overlays/prod/cluster.yaml   # cluster "swiftmed"
kubectl apply -k deploy/k8s/platform/sealed-secrets                 # once per cluster
#   restore the controller key first if you have one (see "Secrets")
kubectl apply -k deploy/k8s/overlays/prod
kubectl -n swiftmed get pods                                         # all Running and 1/1
kubectl -n swiftmed create job seed-$(date +%s) --from=cronjob/seed  # demo data, safe to repeat
```

The images come from `ghcr.io/swiftmeds-bht/<name>:v1.0.0`, pushed by CI. If the
packages are private, add a pull secret first:
`kubectl -n swiftmed create secret docker-registry ghcr-pull --docker-server=ghcr.io --docker-username=<user> --docker-password=<token with read:packages>`
and `imagePullSecrets: [{name: ghcr-pull}]` (or make the packages public).
Without GHCR access, `scripts/k8s-images.sh --tag v1.0.0 --import swiftmed`
builds the same images on the laptop and loads them into the cluster.

**Moving a cluster made with the first layout** (one `deploy/k8s` folder): the
Secret must come from the SealedSecret now, and a StatefulSet's volume size
cannot be changed in place. Keep the pods and data while switching:

```bash
kubectl -n swiftmed delete secret swiftmed-secrets
kubectl -n swiftmed delete statefulset postgres rabbitmq --cascade=orphan
kubectl apply -k deploy/k8s/overlays/prod
```

The existing volumes keep their size (k3s's local-path storage cannot grow a
volume); the prod sizes apply to new clusters.

## A development cluster

```bash
deploy/k8s/overlays/local/up.sh    # cluster "swiftmed-dev" on 9100/9180, images built here, seeded
```

## Secrets

The Secret is committed only in encrypted form, as
`overlays/prod/sealed-secrets.yaml`. Only the cluster's Sealed Secrets controller
can decrypt it.

- **Change a secret**: put all keys of `base/secrets.env.example` in a file
  outside the repository (or run `overlays/local/make-secrets.sh`, which writes the
  gitignored `overlays/local/secrets.env` from `.env`), then
  `deploy/k8s/overlays/prod/seal-secrets.sh <that file>`, commit the new
  `sealed-secrets.yaml`, `kubectl apply -k deploy/k8s/overlays/prod`, and
  `kubectl -n swiftmed rollout restart deployment`. The script refuses `change-me`
  values. (Postgres and RabbitMQ take their passwords only when their volumes are
  created.)
- **Back up the controller key** right after installing the controller. It is
  the only way to decrypt the committed file; keep it out of Git:
  `kubectl -n kube-system get secret -l sealedsecrets.bitnami.com/sealed-secrets-key -o yaml > sealed-secrets-key.yaml`
- **New cluster**: `kubectl apply -f sealed-secrets-key.yaml` before installing the
  controller, then deploy as above. Without the key, re-seal the secrets for the
  new controller.

## Update

- **A release**: push a tag, for example `git tag v1.0.1 && git push origin v1.0.1`;
  CI pushes all images under that tag. Set `newTag` in
  `overlays/prod/kustomization.yaml`, then `kubectl apply -k deploy/k8s/overlays/prod`.
  Pods are replaced one at a time (rolling update).
- **A setting**: edit `base/config.env` and apply. The ConfigMap name carries a
  content hash, so every pod that reads it is rolled automatically.
- **Roll back**: `kubectl -n swiftmed rollout undo deployment/<name>`;
  `kubectl -n swiftmed rollout history deployment/<name>` lists the revisions. For a
  whole release, set the previous `newTag` and apply.

## Scale

```bash
kubectl -n swiftmed get hpa                                  # gateway and search: 2 to 4 pods on CPU
kubectl -n swiftmed scale deployment/inventory --replicas=3  # others by hand
```

## Back up and restore Postgres

The CronJob `postgres-backup` runs `pg_dumpall --clean --if-exists` every night at
02:30 (Asia/Thimphu) and writes `swiftmed-<date>.sql.gz` to the volume
`postgres-backups`, separate from the database's own volume. It keeps the newest 7,
and refuses to store a dump that is too small to be real.

```bash
# A backup now
kubectl -n swiftmed create job backup-now --from=cronjob/postgres-backup
kubectl -n swiftmed logs job/backup-now          # backup: /backups/swiftmed-...sql.gz (1.5M)

# List the backups, or copy one to the laptop
kubectl apply -f deploy/k8s/tools/backup-shell.yaml
kubectl -n swiftmed exec backup-shell -- ls -lh /backups
kubectl -n swiftmed cp backup-shell:/backups/<file> ./<file>
```

**Restore** (the services are stopped while their databases are replaced):

```bash
kubectl apply -f deploy/k8s/tools/backup-shell.yaml
kubectl -n swiftmed scale deployment --all --replicas=0
kubectl -n swiftmed exec backup-shell -- sh -c 'gunzip -c /backups/<file> | psql -q -d postgres'
kubectl apply -k deploy/k8s/overlays/prod          # puts every replica count back
kubectl -n swiftmed delete pod backup-shell
```

`ERROR: current user cannot be dropped` is expected: the dump also re-creates the
user it is restored with. Into a new cluster, deploy first (so Postgres and the
backups volume exist), copy the file in with `kubectl cp`, then follow the same steps.

## Public access

The prod overlay runs two **Cloudflare quick tunnels** (`overlays/prod/tunnel.yaml`,
cloudflared 2026.10.7): one for the SwiftMed web app (patients at `/`, pharmacy
staff at `/pharmacy-portal`; one app since SCRUM-139) and one for the API gateway. Quick tunnels need no Cloudflare account and no domain, and open no port
on the laptop: cloudflared connects out to Cloudflare, which serves HTTPS on
`https://<random-words>.trycloudflare.com` and forwards to Traefik's public
entrypoints. Postgres, RabbitMQ, the gRPC port and Traefik's dashboard stay
unreachable (NetworkPolicies), and the gateway's worker routes need the shared secret.

```bash
deploy/k8s/overlays/prod/tunnel-urls.sh        # prints the site, portal and API URLs
```

A URL stays the same while the `public-tunnel` pod runs and **changes when it
restarts** (a laptop restart, a node drain). After that, print the new URLs and
point the workers at the new API URL (below).

**Cloudflare Workers.** Both workers (`workers/hold-expiry`, every minute, and
`workers/freshness-decay`, hourly) call the gateway's internal routes with the
shared secret. Deploy them to your Cloudflare account, pointed at the current
API URL:

1. Create an API token: dash.cloudflare.com -> My Profile -> API Tokens -> Create
   Token -> template **Edit Cloudflare Workers** -> your account. Keep it secret.
2. In WSL, from the repository:
   ```bash
   export CLOUDFLARE_API_TOKEN=<the token>
   deploy/k8s/overlays/prod/tunnel-urls.sh --deploy-workers
   ```
   This deploys both workers with `GATEWAY_URL` set to the API tunnel and, the
   first time, sets their `WORKER_SHARED_SECRET` from `.env` (the same value the
   cluster uses). Use `--set-secret` to set it again after changing it.
3. Check that they reach the cluster, in Traefik's access log (it sees every
   request; `logs deploy/gateway` reads only one of the two gateway pods):
   ```bash
   kubectl -n swiftmed logs -l app.kubernetes.io/name=traefik --since=5m --tail=-1 \
     | grep -oE '"RequestPath":"/api/v1/internal/[a-z/]+"|"DownstreamStatus":[0-9]+' | paste - -
   ```
   Expect one `/api/v1/internal/expiry/sweep` with status 200 a minute, and the
   freshness call on the hour. 401 means the workers' secret differs from the cluster's
   (`--set-secret`); 530 or 1033 means the API tunnel's URL has changed (run the script again).

`wrangler` needs Node.js in WSL; the tools cache has one
(`export PATH=$HOME/.cache/swiftmed-tools/node/bin:$PATH`), or install Node 20 or newer.

`wrangler login` does not work from WSL here (its browser callback goes to a
port inside WSL that Windows cannot reach), hence the token.

**Limits of quick tunnels** (Cloudflare documents them as for testing):

- **No Server-Sent Events.** The live back-in-stock toast (SCRUM-71) does not
  arrive through the quick tunnel (tested: no heartbeat in 40 s, while the same
  stream directly from the laptop sent one every 15 s). The alert is kept, and
  the watchlist page shows it when the patient opens or refreshes it. Live
  alerts work locally and through a named tunnel.
- The URL changes on restart (see above), there is no uptime guarantee, and
  Cloudflare limits a quick tunnel to 200 requests in flight, plenty for a demo.

**Moving to a named tunnel** once the team has a domain on Cloudflare (fixed
URLs, SSE works): `cloudflared tunnel login` (authorises the domain),
`cloudflared tunnel create swiftmed`, add the public hostnames in the
dashboard (for example `swiftmed.<domain>` -> `http://traefik:3000` and
`api.<domain>` -> `http://traefik:8080`),
store the tunnel token as a sealed secret, and run cloudflared with
`tunnel run --token` instead of `--url`. Then deploy the workers once with the
fixed API URL.

Verified on 18 Nov 2026 through the public URLs, in a phone-sized browser: the
demo patient signed in, searched, saw live stock, reserved 2 units and got a
pickup code; the pharmacist signed in on the public dashboard and collected
with that code; the patient's hold then showed "Collected". The admin account
signed in over the public API (`ROLE_ADMIN`). From the internet, the gRPC port is
unreachable and the worker route without its secret answers 401. Both workers
were deployed with `tunnel-urls.sh --deploy-workers`; Cloudflare's cron then
called `/api/v1/internal/expiry/sweep` through the tunnel every minute
(16:15, 16:16, 16:17 UTC), each answered 200.

### Demo setup: fixed API address and the web app on Vercel (SCRUM-156)

For presentations the site is on **Vercel** (a fixed `*.vercel.app` link for the
report) and talks to this cluster through a **fixed API address** on ngrok's free
static domain: `https://entrench-surging-backtrack.ngrok-free.dev`. The cluster still runs on the
laptop, so the site works whenever the cluster is up.

Once:

1. ngrok: a free account, the static domain above (Domains), pooling allowed.
   Seal its authtoken (hidden prompt; commit the result):
   ```bash
   deploy/k8s/overlays/prod/seal-ngrok.sh
   ```
   and keep `sealed-ngrok.yaml` in the prod kustomization. The `api-fixed`
   container in `public-tunnel` then connects on every start.
2. Workers: `tunnel-urls.sh --deploy-workers` now deploys them with the fixed
   address, so they keep working across restarts.
3. Vercel: import the repository, Root Directory `web/swiftmed`, framework
   Next.js. Environment variable `GATEWAY_URL=https://entrench-surging-backtrack.ngrok-free.dev`
   (Production). `web/swiftmed/vercel.json` runs the functions in Singapore
   (`sin1`), close to the cluster.

Before each demo:

```bash
k3d cluster start swiftmed                                  # ~2 minutes
kubectl -n swiftmed wait --for=condition=Available deploy --all --timeout=600s
curl -s https://entrench-surging-backtrack.ngrok-free.dev/healthz        # {"status":"ok"}
```

Then open the Vercel site, search once (the first search warms the services up)
and log in with the demo accounts. Afterwards: `k3d cluster stop swiftmed`.

Limits: ngrok's free plan caps requests and bandwidth (enough for a demo) and
shows a warning page to browsers that open the API address directly; the web app
calls it from its server, so visitors never see it. On Vercel, several function
instances can refresh the same session at once (`lib/refresh.ts` shares
refreshes only within one process), so a user may occasionally be asked to log
in again.

## Monitoring

`deploy/k8s/monitoring` (namespace `monitoring`) adds metrics and central logs:

| Part | What it does |
|---|---|
| Prometheus v3.15.1 | Scrapes Traefik (requests and latency per service: every gRPC call between services and every outside request passes through it), the kubelet's cAdvisor (CPU, memory, network per pod) and kube-state-metrics. Keeps 7 days on a 5 Gi volume. |
| kube-state-metrics v2.20.0 | Replicas, HPA replicas, restarts, pod phases as metrics. |
| Loki 3.7.9 | The logs of every SwiftMed pod, 7 days, on a 5 Gi volume. |
| Grafana Alloy v1.21.3 | Ships the pod logs to Loki through the Kubernetes API (no access to node files), with the labels `namespace`, `app`, `pod`, `container` and `level` (from the services' JSON logs). Traefik's access log is JSON too. |
| Grafana 13.2.5 | Prometheus and Loki as data sources and the "SwiftMed overview" dashboard (`monitoring/dashboards/swiftmed-overview.json`), all provisioned. |

```bash
kubectl apply -k deploy/k8s/monitoring
kubectl -n monitoring port-forward --address 0.0.0.0 svc/grafana 3300:3000
#   http://localhost:3300 in WSL; from a Windows browser use WSL's address,
#   http://$(hostname -I | cut -d' ' -f1):3300
```

Grafana's admin password is a SealedSecret (`monitoring/grafana-admin-sealed.yaml`);
`monitoring/make-grafana-secret.sh` makes a new random one, seals it, and writes the
password to `~/swiftmed-grafana-admin.txt` (readable only by you). Anonymous access
is off. Nothing in the namespace is exposed outside the cluster, and its own
NetworkPolicies only let Grafana reach Prometheus and Loki, Alloy reach Loki, and
Prometheus reach kube-state-metrics. In `swiftmed`, only Prometheus may read
Traefik's metrics port.

The dashboard shows requests per second and p95 latency per service, the gateway's
and web apps' responses by HTTP status and the gateway's 5xx share, CPU and memory
per pod, replicas (including the HPAs), restarts in the last hour, log lines per
app and level, and the latest warnings and errors. Useful Loki queries in
**Explore**:

```
{namespace="swiftmed", app="holds"}                          one service's logs
{namespace="swiftmed", level="ERROR"}                        every error
{namespace="swiftmed", app="gateway"} | json | status >= 400 failed API requests
{namespace="swiftmed"} | json | request_id="<id>"           one request, by its X-Request-ID
```

Limit: gRPC calls always count as HTTP 200 at Traefik (their status travels in
HTTP/2 trailers), so per-service error rates are visible only at the gateway's
REST API. Counting gRPC status codes needs a metrics interceptor in `pkg/grpcx`
exporting Prometheus metrics from every service; that is a later step.

Verified on the laptop (18 Nov 2026): all 5 scrape targets up; 60 searches showed
as 60 calls each to search, catalog, geo, inventory, ranking, freshness and
pharmacy-registry (one search's fan-out); the dashboard's queries returned data
for 10 services, 31 pods and both HPAs; Loki held the logs of 26 apps, and
`| json | status >= 400` found the 404 sent on purpose; both data sources
healthy; anonymous requests refused.

## Show in the report

Measured on the team laptop (k3d, 20 CPUs, 8 GB for WSL), 18 Nov 2026, prod
overlay. Each test ran searches through the gateway from 4 parallel clients the
whole time and counted the status codes.

| Demo | How | Result |
|---|---|---|
| Self-healing | delete one search pod | replacement Ready in 17 s; **5,251 searches, all 200** |
| No single point of failure | delete one of the two Traefik pods, twice | **8,401 and 8,412 searches, all 200** (before the `preStop` pause on Traefik: 8,328 failures in ~20 s) |
| Traefik active-active under load (SCRUM-135) | k6 `search.js`, 100 users for 2 min (below); at 60 s delete one Traefik pod, 41 s later the other original one | **6,976 searches, 0 failed**, p95 14 ms, max 121 ms, all thresholds pass. Both replacement pods served (740 and 721 searches, 6,522 and 5,493 gRPC calls) |
| Autoscaling | load on search; `kubectl -n swiftmed get hpa`, `describe hpa search` | gateway and search HPAs 1 -> **4 pods**, `SuccessfulRescale ... cpu ... above target` |
| Rolling update + rollback | `kubectl -n swiftmed set env deployment/search LOG_LEVEL=debug`, `rollout undo` | **3,061 searches, all 200** |
| Network policies | throw-away pods trying each connection (`nc -z`) | unlabelled pod -> Postgres, RabbitMQ, search, gateway: **blocked**; -> Traefik: allowed. Postgres client -> Postgres: allowed, -> RabbitMQ: blocked. Other namespace -> Postgres, Traefik gRPC: **blocked**; -> Traefik :8080: allowed |
| Backup and restore | marker row, backup, drop it, restore as above | backup 1.5 MB; **marker row back**, all 22 accounts intact, search 200 |
| End to end | Cypress journey specs (`tests/e2e`) against the cluster | **5 passing** (the expiry spec skips itself with 2-hour holds) |

Load with k6 (`tests/load/search.js`), from the repository root. The script reads
the seed medicines (`deploy/seed`), so mount the whole repository:

```bash
docker run --rm --network host --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work/tests/load \
  -e BASE_URL=http://localhost:9080 -e VUS=100 -e HOLD=2m grafana/k6:1.0.0 run search.js
kubectl -n swiftmed get hpa -w          # HPA demo, in another terminal
# Active-active demo, during the run: delete ONE Traefik pod (a label selector
# alone would delete both at once).
kubectl -n swiftmed delete "$(kubectl -n swiftmed get pods -l app.kubernetes.io/name=traefik -o name | head -n 1)" --wait=false
```

The summary lands in `tests/load/results/search.md`.

What the tests found, and fixed: a pod being deleted dropped calls in the instant
before Traefik removed it (fixed with the `preStop` pause, also on Traefik itself);
the first backup job could not reach Postgres in its first second (network
policies reach a new pod a moment later) and, without `pipefail`, stored an empty
file as a success (fixed: it waits for Postgres and checks the dump's size).

## Troubleshooting

| Symptom | Look at |
|---|---|
| A pod is not Ready | `kubectl -n swiftmed describe pod <pod>` (events), `kubectl -n swiftmed logs <pod> --all-containers` |
| `Init:` stays for long | Postgres or RabbitMQ is not up: `kubectl -n swiftmed get pods postgres-0 rabbitmq-0` |
| `CreateContainerConfigError` | a missing key in the Secret or `config.env`; the event names it |
| The Secret does not appear | `kubectl -n swiftmed describe sealedsecret swiftmed-secrets` (a Secret not made by the controller blocks it: delete that Secret) |
| A connection times out inside the cluster | a NetworkPolicy: does the client pod have the `swiftmed/*-client` label it needs? |
| A gRPC call fails | `kubectl -n swiftmed get ingress`, `kubectl -n swiftmed logs deploy/traefik`, Traefik's dashboard: `kubectl -n swiftmed port-forward deploy/traefik 8081` -> http://localhost:8081/dashboard/ |
| HPA shows `<unknown>` | metrics are a minute behind a new pod; `kubectl top pods -n swiftmed` |

## Tear down

```bash
kubectl delete -k deploy/k8s/overlays/prod   # remove SwiftMed, keep the cluster
k3d cluster delete swiftmed                  # remove everything, including data and backups
```

## Fallback

If the cluster is not usable before a demo, Docker Compose (`make up && make seed`)
runs the same system (risk R9, SCRUM-100).

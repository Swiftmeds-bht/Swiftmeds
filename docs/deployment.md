# Deployment runbook (Kubernetes)

SwiftMed is deployed on **Kubernetes (k3s)**; Docker Compose stays the local
development stack ([ADR 0005](adr/0005-docker-compose-main-stack-kubernetes-demo.md)).
The deployment host is the **team laptop**, where k3s runs in Docker through
[k3d](https://k3d.io). The manifests in [`deploy/k8s/`](../deploy/k8s) are plain
Kustomize and work on any k3s or Kubernetes cluster, so moving to a cloud VM
later means installing k3s there and running the same `kubectl apply -k`.

| What | Where |
|---|---|
| Patient app | http://localhost:9000 |
| Pharmacy dashboard | http://localhost:9001 |
| API gateway | http://localhost:9080 (`/healthz`) |
| Public URL (Cloudflare Tunnel) | SCRUM-75 |
| Demo accounts (password `swiftmeds-demo`) | `patient@demo.swiftmeds.test`, `demo-tim-001@demo.swiftmeds.test` (pharmacist of Demo Pharmacy Thimphu 01) |

## What runs

```
                 laptop :9000 :9001 :9080
                          |
              k3d load balancer -> Service traefik-public
                          |
                       Traefik  (entrypoints web/patient/dashboard, grpc :50051)
           /          |           \                      \
   patient-app   pharmacy-     gateway  --gRPC-->  Traefik grpc :50051 --by :authority--> 19 gRPC services
                 dashboard                                                                     |
                                                                             Postgres (StatefulSet, 2Gi)
                                                                             RabbitMQ (StatefulSet, 1Gi)
```

- **20 services + 2 web apps**: one Deployment, Service and Ingress route each
  (`deploy/k8s/services/*.yaml`, `gateway.yaml`, `web.yaml`).
- **Traefik inside the cluster** (`traefik.yaml`), configured like the Compose
  one. Services still call `traefik:50051` with the callee's name as
  `:authority`, so no service setting differs from Compose. Traefik sends each
  gRPC call to a pod directly, so replicas share the load (a plain Service would
  pin each HTTP/2 connection to one pod). k3s's bundled Traefik is switched off.
- **Postgres and RabbitMQ** as StatefulSets with persistent volumes. Postgres
  creates one database per service from `deploy/postgres/init/01-databases.sql`
  (the same file as Compose, carried by the `seed` image).
- **Settings** in the ConfigMap `swiftmed-config` (from `deploy/k8s/config.env`);
  **secrets** in the Secret `swiftmed-secrets` (from `deploy/k8s/secrets.env`,
  never committed). Each pod gets only the secrets it uses.
- **Probes**: gRPC health checks (`grpc.health.v1`) on every gRPC service, which
  report ready only after migrations ran; `/healthz` on the gateway; HTTP on the
  web apps. Init containers wait for Postgres and RabbitMQ, so a first deploy
  does not crash-loop. A 5-second `preStop` pause lets Traefik drop a pod before
  it stops, so deletes and rolling updates lose no requests.
- **Requests and limits** on every container; **HPAs** on Search and the gateway
  (CPU 70 %, 1 to 4 pods), using the metrics-server that comes with k3s.
- Pods run as non-root with all capabilities dropped; the Go services also have
  a read-only root filesystem.

## Install (once per laptop)

Needs Docker (with WSL 2 on Windows) and about 4 GB of free memory. k3d and
kubectl are single binaries:

```bash
mkdir -p ~/.local/bin
curl -fsSLo ~/.local/bin/kubectl https://dl.k8s.io/release/v1.36.6/bin/linux/amd64/kubectl
curl -fsSLo ~/.local/bin/k3d https://github.com/k3d-io/k3d/releases/download/v5.9.0/k3d-linux-amd64
chmod +x ~/.local/bin/kubectl ~/.local/bin/k3d
cp .env.example .env          # and change the secrets
```

## Deploy

One command, from nothing to a seeded deployment (safe to run again):

```bash
deploy/k8s/local/up.sh
```

It creates the k3d cluster (`deploy/k8s/local/cluster.yaml`, k3s v1.36.6),
builds the 23 images and loads them into the cluster, writes `secrets.env` from
`.env`, applies the manifests, waits for every rollout, and runs the seed Job.
On the team laptop a first run took about 10 minutes, mostly image builds.

The same steps by hand:

```bash
k3d cluster create --config deploy/k8s/local/cluster.yaml
scripts/k8s-images.sh --tag dev --import swiftmed      # or --push, see "Images"
deploy/k8s/make-secrets.sh                              # deploy/k8s/secrets.env from .env
kubectl apply -k deploy/k8s
kubectl -n swiftmed get pods                            # all Running and 1/1
kubectl -n swiftmed delete job seed --ignore-not-found && kubectl apply -k deploy/k8s/seed-job
```

Check it: `curl http://localhost:9080/healthz`, then open the patient app. The
Cypress journey tests (`tests/e2e`, SCRUM-72) also pass against the cluster.

## Update

- **A new version of a service**: build and load (or push) the image, then roll
  it out. With the `:dev` tag on the laptop:

  ```bash
  scripts/k8s-images.sh --tag dev --import swiftmed
  kubectl -n swiftmed rollout restart deployment/search
  kubectl -n swiftmed rollout status deployment/search
  ```

  For a release, set `newTag` for the images in `deploy/k8s/kustomization.yaml`
  (for example `v1.0.0`) and `kubectl apply -k deploy/k8s`.
- **A setting**: edit `deploy/k8s/config.env` and `kubectl apply -k deploy/k8s`.
  The ConfigMap name carries a content hash, so every pod that reads it is
  rolled automatically.
- **A secret**: edit `deploy/k8s/secrets.env`, `kubectl apply -k deploy/k8s`,
  then `kubectl -n swiftmed rollout restart deployment`. (The Postgres and
  RabbitMQ passwords are set only when their volumes are created.)
- **Roll back** the last rollout: `kubectl -n swiftmed rollout undo deployment/search`.
  `kubectl -n swiftmed rollout history deployment/search` lists the revisions.

## Scale

```bash
kubectl -n swiftmed get hpa                                 # Search and gateway scale on CPU by themselves
kubectl -n swiftmed scale deployment/inventory --replicas=2 # others by hand
```

Traefik spreads gRPC calls over every ready pod, so new replicas take load at once.

## Back up and restore Postgres

```bash
# Back up every database to one file on the laptop
kubectl -n swiftmed exec postgres-0 -- sh -c 'pg_dumpall -U "$POSTGRES_USER"' > swiftmed-$(date +%F).sql
# Restore into a fresh cluster (before the services write anything)
kubectl -n swiftmed exec -i postgres-0 -- sh -c 'psql -U "$POSTGRES_USER" -d postgres' < swiftmed-2026-11-18.sql
```

The data lives in the PersistentVolumeClaim `data-postgres-0`, which survives pod
restarts and `kubectl apply`, but not `k3d cluster delete`.

## Show in the report

Measured on the team laptop (k3d, 20 CPUs, 8 GB for WSL), 18 Nov 2026. Each test
ran searches through the gateway (`/api/v1/search`) from 4 parallel clients the
whole time and counted the status codes.

| Demo | Commands | Result |
|---|---|---|
| **Self-healing** | `kubectl -n swiftmed delete pod <one search pod>` | The ReplicaSet started a replacement, Ready in 17 s. **5,251 searches, all 200.** |
| **Autoscaling** | load on search (below), then `kubectl -n swiftmed get hpa` and `kubectl -n swiftmed describe hpa search` | Both HPAs went from 1 to **4 pods**: `SuccessfulRescale ... New size: 4; reason: cpu resource utilization (percentage of request) above target`; the gateway showed 110 %/70 %. |
| **Rolling update + rollback** | `kubectl -n swiftmed set env deployment/search LOG_LEVEL=debug`, `rollout status`, `rollout undo deployment/search` | **3,061 searches during the update and the rollback, all 200.** |

Load for the HPA demo with k6 (SCRUM-76's script), on the cluster's network:

```bash
docker run --rm --network k3d-swiftmed -v "$PWD/tests/load:/load" -w /load \
  -e BASE_URL=http://k3d-swiftmed-serverlb:8080 grafana/k6 run search.js
kubectl -n swiftmed get hpa -w          # in another terminal
```

Without k6, a simple loop does it too:
`while true; do curl -s -o /dev/null 'http://localhost:9080/api/v1/search?q=paracetamol&lat=27.4728&lng=89.6390&radius_m=5000'; done`
in a few terminals.

A note from the first self-healing test: before the `preStop` pause was added, a
search sent in the instant between a pod stopping and Traefik dropping it failed
with 500. The pause closed that gap; the table above is with it.

## Images

`scripts/k8s-images.sh` builds all 23 images with the existing Dockerfiles: the
20 services, the two web apps and `seed` (psql plus the demo fixtures).

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-user> --password-stdin  # token with write:packages
scripts/k8s-images.sh --tag v1.0.0 --push                                       # ghcr.io/swiftmeds-bht/<name>:v1.0.0
scripts/k8s-images.sh --tag v1.0.0 --platform linux/arm64 --push                # for an ARM host
```

If the packages are private, the cluster needs a pull secret:
`kubectl -n swiftmed create secret docker-registry ghcr-pull --docker-server=ghcr.io --docker-username=<user> --docker-password=<token with read:packages>`
and `imagePullSecrets: [{name: ghcr-pull}]` on the pods (or make the packages public).

## Troubleshooting

| Symptom | Look at |
|---|---|
| A pod is not Ready | `kubectl -n swiftmed describe pod <pod>` (events), `kubectl -n swiftmed logs <pod> --all-containers` |
| `Init:` stays for long | Postgres or RabbitMQ is not up: `kubectl -n swiftmed get pods postgres-0 rabbitmq-0` |
| `CreateContainerConfigError` | a missing key in `secrets.env` or `config.env`; the event names it |
| A gRPC call fails with 404/502 | the Traefik route: `kubectl -n swiftmed get ingress`, `kubectl -n swiftmed logs deploy/traefik`, Traefik's dashboard with `kubectl -n swiftmed port-forward deploy/traefik 8081` -> http://localhost:8081/dashboard/ |
| HPA shows `<unknown>` | metrics are a minute behind a new pod; `kubectl top pods -n swiftmed` |

## Tear down

```bash
kubectl delete -k deploy/k8s     # remove SwiftMed, keep the cluster
k3d cluster delete swiftmed      # remove everything, including the data
```

## Fallback

If the cluster is not usable before a demo, Docker Compose (`make up && make seed`)
runs the same system (risk R9, SCRUM-100).

# Deployment runbook (Kubernetes)

SwiftMed is deployed on **Kubernetes (k3s)**; Docker Compose stays the local
development stack ([ADR 0005](adr/0005-docker-compose-main-stack-kubernetes-demo.md)).
The deployment runs on the **team laptop**, where k3s runs in Docker through
[k3d](https://k3d.io). The manifests are plain Kustomize, so the same overlay
deploys to any k3s or Kubernetes cluster (a cloud VM later: install k3s there
and run the same commands).

| What | Where |
|---|---|
| Patient app | http://localhost:9000 |
| Pharmacy dashboard | http://localhost:9001 |
| API gateway | http://localhost:9080 (`/healthz`) |
| Public URL (Cloudflare Tunnel) | SCRUM-75 |
| Demo accounts (password `swiftmeds-demo`) | `patient@demo.swiftmeds.test`, `demo-tim-001@demo.swiftmeds.test` (pharmacist of Demo Pharmacy Thimphu 01) |

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
  tools/backup-shell.yaml    a helper pod for listing and restoring backups
```

CI checks that `base` and both overlays render and match the Kubernetes schemas
(`kustomize build | kubeconform -strict`, job "Kubernetes manifests"), so a
broken YAML file fails the PR. Pushing a `v*` tag builds the 23 images and pushes
them to `ghcr.io/swiftmeds-bht` (workflow "Release images").

## What runs

```
                 laptop :9000 :9001 :9080   (Cloudflare Tunnel, SCRUM-75)
                          |
              k3d load balancer -> Service traefik-public
                          |
                 Traefik x2 (web/patient/dashboard entrypoints, grpc :50051)
           /          |           \                      \
   patient-app   pharmacy-     gateway x2+ --gRPC--> Traefik grpc :50051 --by :authority--> 19 gRPC services
                 dashboard                                                 (search x2+, holds x2, inventory x2)
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
- **NetworkPolicies** (`base/networkpolicies.yaml`): everything is denied unless
  allowed. From outside, only Traefik's public ports (8080, 3000, 3001); inside,
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
deploy/k8s/overlays/local/up.sh    # cluster "swiftmed-dev" on 9100/9101/9180, images built here, seeded
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

## Show in the report

Measured on the team laptop (k3d, 20 CPUs, 8 GB for WSL), 18 Nov 2026, prod
overlay. Each test ran searches through the gateway from 4 parallel clients the
whole time and counted the status codes.

| Demo | How | Result |
|---|---|---|
| Self-healing | delete one search pod | replacement Ready in 17 s; **5,251 searches, all 200** |
| No single point of failure | delete one of the two Traefik pods, twice | **8,401 and 8,412 searches, all 200** (before the `preStop` pause on Traefik: 8,328 failures in ~20 s) |
| Autoscaling | load on search; `kubectl -n swiftmed get hpa`, `describe hpa search` | gateway and search HPAs 1 -> **4 pods**, `SuccessfulRescale ... cpu ... above target` |
| Rolling update + rollback | `kubectl -n swiftmed set env deployment/search LOG_LEVEL=debug`, `rollout undo` | **3,061 searches, all 200** |
| Network policies | throw-away pods trying each connection (`nc -z`) | unlabelled pod -> Postgres, RabbitMQ, search, gateway: **blocked**; -> Traefik: allowed. Postgres client -> Postgres: allowed, -> RabbitMQ: blocked. Other namespace -> Postgres, Traefik gRPC: **blocked**; -> Traefik :8080: allowed |
| Backup and restore | marker row, backup, drop it, restore as above | backup 1.5 MB; **marker row back**, all 22 accounts intact, search 200 |
| End to end | Cypress journey specs (`tests/e2e`) against the cluster | **5 passing** (the expiry spec skips itself with 2-hour holds) |

Load for the HPA demo with k6 (SCRUM-76's script), on the cluster's network:

```bash
docker run --rm --network k3d-swiftmed -v "$PWD/tests/load:/load" -w /load \
  -e BASE_URL=http://k3d-swiftmed-serverlb:8080 grafana/k6 run search.js
kubectl -n swiftmed get hpa -w          # in another terminal
```

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

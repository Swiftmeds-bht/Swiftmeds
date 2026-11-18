# syntax=docker/dockerfile:1
# The "seed" image (SCRUM-108): psql plus the demo fixtures and the database
# init script, from the same files Docker Compose uses, so nothing is copied
# into deploy/k8s. Used by the seed and backup CronJobs (base/seed.yaml, base/backup.yaml), by Postgres's
# init container (the CREATE DATABASE script) and by the wait-for init containers
# (pg_isready, nc).
#
# Build from the deploy/ folder (the repository's .dockerignore leaves deploy/ out):
#   docker build -f deploy/k8s/base/seed/Dockerfile -t ghcr.io/swiftmeds-bht/seed deploy
FROM postgres:16-alpine
COPY seed/ /opt/swiftmed/seed/
COPY postgres/init/ /opt/swiftmed/postgres-init/
# Not the Postgres server entrypoint: the Job and init containers give the command.
ENTRYPOINT []
CMD ["sh", "/opt/swiftmed/seed/seed.sh"]

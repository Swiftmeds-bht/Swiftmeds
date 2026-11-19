# syntax=docker/dockerfile:1
# Build from the REPO ROOT (the service needs pkg/ and proto/gen/go/):
#   docker build -f services/geo/Dockerfile -t swiftmed/geo .

# ---- build stage ----
FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOWORK=off
WORKDIR /src

# Copy only module files first, so dependencies are cached until a go.mod changes.
COPY pkg/go.mod pkg/go.sum pkg/
COPY proto/gen/go/go.mod proto/gen/go/go.sum proto/gen/go/
COPY services/geo/go.mod services/geo/go.sum services/geo/
RUN cd services/geo && go mod download

COPY pkg/ pkg/
COPY proto/gen/go/ proto/gen/go/
COPY services/geo/ services/geo/
RUN cd services/geo && go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- run stage: static binary only, no shell, runs as non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 50062
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]

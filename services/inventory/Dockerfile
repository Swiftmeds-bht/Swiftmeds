# syntax=docker/dockerfile:1
# Build from the REPO ROOT (the service needs pkg/ and proto/gen/go/):
#   docker build -f services/inventory/Dockerfile -t swiftmed/inventory .

# ---- build stage ----
FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOWORK=off
WORKDIR /src

# Copy only module files first, so dependencies are cached until a go.mod changes.
COPY pkg/go.mod pkg/go.sum pkg/
COPY proto/gen/go/go.mod proto/gen/go/go.sum proto/gen/go/
COPY services/inventory/go.mod services/inventory/go.sum services/inventory/
RUN cd services/inventory && go mod download

COPY pkg/ pkg/
COPY proto/gen/go/ proto/gen/go/
COPY services/inventory/ services/inventory/
RUN cd services/inventory && go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- run stage: static binary only, no shell, runs as non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 50056
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]

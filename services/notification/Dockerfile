# Build from the repository root:
# docker build -f services/notification/Dockerfile -t swiftmed/notification .
FROM golang:1.25-alpine AS build
ENV CGO_ENABLED=0 GOWORK=off
WORKDIR /src
COPY pkg/go.mod pkg/go.sum pkg/
COPY proto/gen/go/go.mod proto/gen/go/go.sum proto/gen/go/
COPY services/notification/go.mod services/notification/go.sum services/notification/
RUN cd services/notification && go mod download
COPY pkg/ pkg/
COPY proto/gen/go/ proto/gen/go/
COPY services/notification/ services/notification/
RUN cd services/notification && go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 50054
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]

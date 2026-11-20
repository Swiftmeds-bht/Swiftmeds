# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOWORK=off
WORKDIR /src
COPY pkg/go.mod pkg/go.sum pkg/
COPY proto/gen/go/go.mod proto/gen/go/go.sum proto/gen/go/
COPY services/user-profile/go.mod services/user-profile/
RUN cd services/user-profile && go mod download
COPY pkg/ pkg/
COPY proto/gen/go/ proto/gen/go/
COPY services/user-profile/ services/user-profile/
RUN cd services/user-profile && go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 50052
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]

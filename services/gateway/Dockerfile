FROM golang:1.25 AS build
WORKDIR /src
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
RUN cd services/gateway && go mod download
COPY services/gateway ./services/gateway
RUN cd services/gateway && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gateway ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /gateway /gateway
EXPOSE 8080
ENTRYPOINT ["/gateway"]

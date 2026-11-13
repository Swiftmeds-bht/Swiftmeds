FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /analytics ./services/analytics/cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /analytics /analytics
ENTRYPOINT ["/analytics"]

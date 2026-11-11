// Package saga holds the reservation saga integration tests (SCRUM-56).
//
// They start Postgres and RabbitMQ in containers (testcontainers), build and run
// the real Holds and Inventory services against them, and drive the saga over
// gRPC the way the gateway and the Expiry service do. The tests only build with
// the "integration" tag and need Docker:
//
//	cd tests/integration && go test -tags integration -race -count=1 ./...
package saga

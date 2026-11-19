module github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync

go 1.26.0

require (
	github.com/Swiftmeds-bht/Swiftmeds/pkg v0.0.0
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go v0.0.0
	github.com/jackc/pgx/v5 v5.9.2
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
)

replace (
	github.com/Swiftmeds-bht/Swiftmeds/pkg => ../../pkg
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go => ../../proto/gen/go
)

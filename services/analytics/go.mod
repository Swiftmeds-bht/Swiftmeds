module github.com/Swiftmeds-bht/Swiftmeds/services/analytics

go 1.25.0

require (
	github.com/Swiftmeds-bht/Swiftmeds/pkg v0.0.0
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go v0.0.0
	github.com/jackc/pgx/v5 v5.7.6
	github.com/rabbitmq/amqp091-go v1.15.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

replace (
	github.com/Swiftmeds-bht/Swiftmeds/pkg => ../../pkg
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go => ../../proto/gen/go
)

.PHONY: build test generate-proto generate-graphql migrate migrate-up migrate-drop migrate-reset migrate-all migrate-reset-all run-auth run-cart run-gateway docker-up docker-down docker-build infra-up infra-down

# Build all binaries
build:
	go build -o ./bin/migrate.exe ./cmd/migrate
	go build -o ./bin/auth-service.exe ./services/auth-service/cmd/server
	go build -o ./bin/cart-service.exe ./services/cart-service/cmd/server
	go build -o ./bin/api-gateway.exe ./gateway/api-gateway/cmd/server

# Run all tests
test:
	go test -v ./pkg/... ./gateway/api-gateway/...

# Generate protobuf code for all services
generate-proto:
	protoc -I. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative contracts/protobuf/auth/auth.proto contracts/protobuf/cart/cart.proto contracts/protobuf/category/category.proto contracts/protobuf/delivery/delivery.proto contracts/protobuf/inventory/inventory.proto contracts/protobuf/merchant/merchant.proto contracts/protobuf/notification/notification.proto contracts/protobuf/order/order.proto contracts/protobuf/payment/payment.proto contracts/protobuf/product/product.proto contracts/protobuf/rating/rating.proto contracts/protobuf/return/return.proto contracts/protobuf/store/store.proto contracts/protobuf/user/user.proto

# Docker compose commands
docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-build:
	docker compose build

infra-up:
	docker compose up -d postgres redis kafka

infra-down:
	docker compose stop postgres redis kafka

# Run migration for a specific service (e.g. make migrate SERVICE=auth or make migrate-up SERVICE=auth)
migrate:
	go run cmd/migrate/main.go -service=$(SERVICE) -action=up

migrate-up:
	go run cmd/migrate/main.go -service=$(SERVICE) -action=up

# Drop all tables for a specific service (e.g. make migrate-drop SERVICE=auth)
migrate-drop:
	go run cmd/migrate/main.go -service=$(SERVICE) -action=drop

# Reset (drop + migrate) for a specific service (e.g. make migrate-reset SERVICE=auth)
migrate-reset:
	go run cmd/migrate/main.go -service=$(SERVICE) -action=reset

# Run migration for ALL services
migrate-all:
	go run cmd/migrate/main.go -service=all -action=up

# Drop all tables for ALL services
migrate-drop-all:
	go run cmd/migrate/main.go -service=all -action=drop

# Reset (drop + migrate) for ALL services
migrate-reset-all:
	go run cmd/migrate/main.go -service=all -action=reset

# Run auth-service server
run-auth:
	go run services/auth-service/cmd/server/main.go

# Run cart-service server
run-cart:
	go run services/cart-service/cmd/server/main.go

# Run api-gateway server
run-gateway:
	go run gateway/api-gateway/cmd/server/main.go

# Generate GraphQL code for api-gateway
generate-graphql gqlgen gql-gen:
	cd gateway/api-gateway && go run github.com/99designs/gqlgen generate

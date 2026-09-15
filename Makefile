.PHONY: build test migrate migrate-up migrate-drop migrate-reset migrate-all migrate-reset-all run-auth

# Build all binaries
build:
	go build -o ./bin/migrate.exe ./cmd/migrate
	go build -o ./bin/auth-service.exe ./services/auth-service/cmd/server

# Run all tests
test:
	go test -v ./pkg/...

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

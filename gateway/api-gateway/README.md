# GoCart API Gateway Service

The **API Gateway** provides a unified GraphQL API endpoint for GoCart microservices, routing GraphQL operations to underlying gRPC microservices (`user-service`, `product-service`, `cart-service`, `order-service`).

---

## 1. Quick Start & Code Generation

### Run gqlgen Code Generator
Whenever you update `internal/graphql/schema/schema.graphqls`, regenerate GraphQL execution runtimes and models:

```bash
# From gateway/api-gateway
go run github.com/99designs/gqlgen generate
```

### Build & Run Tests
```bash
go test ./...
go build ./...
```

---

## 2. Directory Structure

```
gateway/api-gateway/
├── gqlgen.yml                  # gqlgen configuration file
├── cmd/
│   └── server/                 # Gateway HTTP server entrypoint
├── internal/
│   ├── config/                 # Gateway configuration
│   ├── graphql/                # GraphQL engine, resolvers, mappers & schema
│   │   ├── schema/
│   │   │   └── schema.graphqls # GraphQL SDL schema
│   │   ├── generated/          # Auto-generated gqlgen runtime (DO NOT EDIT)
│   │   ├── model/              # Auto-generated GraphQL Go models (DO NOT EDIT)
│   │   ├── mappers/            # Protobuf -> GraphQL map converters (package maps)
│   │   ├── mutations/          # Mutation resolvers (package mutation)
│   │   └── resolvers/          # Query resolvers & schema stubs (package resolvers)
│   └── grpc/                   # gRPC client wrappers for microservices
└── tests/                      # Gateway integration & end-to-end tests
```

---

## 3. Documentation

For detailed step-by-step instructions on creating GraphQL resolvers from scratch, schema design, and gRPC mapping, see:
- [docs/graphql-development.md](../../docs/graphql-development.md)

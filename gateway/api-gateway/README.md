# GoCart API Gateway Service

The **API Gateway** provides a unified GraphQL API endpoint for GoCart microservices.

---

## Code Generation Command

Whenever you update `internal/graphql/schema/schema.graphqls`, run:

```bash
# From repository root:
make generate-graphql

# Or from gateway/api-gateway directory:
go run github.com/99designs/gqlgen generate
```

---

## Directory Layout

```
gateway/api-gateway/internal/graphql/
├── schema/schema.graphqls    # GraphQL Schema definition
├── generated/generated.go    # Auto-generated runtime (DO NOT EDIT)
├── model/models_gen.go       # Auto-generated models (DO NOT EDIT)
├── mappers/                  # Protobuf -> GraphQL mappers (package maps)
├── mutations/                # Mutation implementations (package mutation)
└── resolvers/                # Query & schema resolvers (package resolvers)
```

# GraphQL Development Quick Guide

## 1. Makefile Auto-Generation Command

To auto-generate GraphQL files from `schema.graphqls`, run:

```bash
make generate-graphql
```
*(Aliases: `make gqlgen` or `make gql-gen`)*

---

## 2. Directory Layout

```
gateway/api-gateway/internal/graphql/
├── schema/schema.graphqls    # GraphQL Schema definition
├── generated/generated.go    # Auto-generated runtime (DO NOT EDIT)
├── model/models_gen.go       # Auto-generated models (DO NOT EDIT)
├── mappers/                  # Protobuf -> GraphQL mappers (package maps)
├── mutations/                # Mutation implementations (package mutation)
└── resolvers/                # Query & schema resolvers (package resolvers)
```

---

## 3. Quick Workflow (Adding New Query / Mutation)

1. **Update Schema**: Edit `gateway/api-gateway/internal/graphql/schema/schema.graphqls`.
2. **Generate Code**: Run `make generate-graphql`.
3. **Implement Logic**:
   - Add Protobuf mappers in `internal/graphql/mappers/`.
   - Implement resolver functions in `internal/graphql/resolvers/` using `r.Clients`.
4. **Test**: Run `cd gateway/api-gateway && go test ./...`.

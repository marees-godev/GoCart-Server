# GoCart AI Guidelines & Coding Standards

- **Independent Services**: Every service under `services/` is fully independent with its own `.env`, config, migrations, and database connection.
- **No Shared Databases**: Services must never access another service's database directly.
- **Go Module Naming**: Root is `github.com/marees-godev/GoCart-Server`. Service modules are `github.com/marees-godev/GoCart-Server/services/<service-name>`.
- **Database Driver**: Use `jackc/pgx/v5` and `pgxpool`.
- **Migrations**: Single `.sql` file per migration in `services/<service-name>/migrations/` (e.g. `000001_init.sql`). No `.up.sql`/`.down.sql` pairing.
- **Service Protocols**: Microservices use gRPC for internal business logic. The microservice HTTP server is strictly for observability (`/health`, `/ready`, `/metrics`), never for direct business REST endpoints.
- **Gateway Service Isolation**: In the API Gateway, prevent merge conflicts and PR blocking by creating dedicated service-specific files for schemas, routes, handlers, and resolvers (e.g. `<service>.graphql`, `<service>_routes.go`, `<service>.resolvers.go`) instead of putting everything into a single shared file.
- **Enum Types**: Always create explicit PostgreSQL `ENUM` types for fixed categorical columns (e.g. statuses, types, roles, categories) instead of generic `VARCHAR` or `TEXT`.
- **Database Normalization**: Every service must strictly adhere to relational database normalization (3NF). Maintain dedicated relational tables with foreign keys and cascade rules instead of dumping structured relational sub-entities into JSON/JSONB or denormalized text columns.
- **Minimal Comments**: Keep code clean, concise, and avoid unnecessary comments.
- **Security**: Never leak credentials or sensitive values in logs.


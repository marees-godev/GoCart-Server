# GoCart Project Rules & AI Guidelines

## Core Architecture
1. **Independent Microservices**:
   - Each service in `services/<service-name>` is completely self-contained with its own `.env`, config, migrations, and database connection.
   - Cross-service database access is strictly prohibited. Services communicate only via APIs/events.
   - Module naming convention: `github.com/marees-godev/GoCart-Server/services/<service-name>`. Root module: `github.com/marees-godev/GoCart-Server`.

2. **Database & Persistence**:
   - Primary persistence: PostgreSQL using `jackc/pgx/v5` and `pgxpool`.
   - Each service owns its database and maintains its own connection pool lifecycle.
   - Shared DB utilities are located in `pkg/database`.

3. **Database Migrations**:
   - Single source of truth per migration: use sequentially numbered single `.sql` files in `services/<service-name>/migrations/` (e.g. `000001_init.sql`, `000002_add_feature.sql`).
   - Do NOT use split `.up.sql` and `.down.sql` files.
   - Migrations are tracked via `schema_migrations` table and executed on service boot if `DB_AUTO_MIGRATE=true`.

4. **Code Quality & Style**:
   - Minimal comments: write self-documenting code. Avoid excessive docstrings, verbose explanations, or obvious comments.
   - Proper context propagation (`ctx context.Context`) and timeout handling.
   - Structured logging using standard library `log/slog`.
   - Never log plain text passwords or secrets. Always sanitize connection strings.

# GoCart AI Guidelines & Coding Standards

- **Independent Services**: Every service under `services/` is fully independent with its own `.env`, config, migrations, and database connection.
- **No Shared Databases**: Services must never access another service's database directly.
- **Go Module Naming**: Root is `github.com/marees-godev/GoCart-Server`. Service modules are `github.com/marees-godev/GoCart-Server/services/<service-name>`.
- **Database Driver**: Use `jackc/pgx/v5` and `pgxpool`.
- **Migrations**: Single `.sql` file per migration in `services/<service-name>/migrations/` (e.g. `000001_init.sql`). No `.up.sql`/`.down.sql` pairing.
- **Minimal Comments**: Keep code clean, concise, and avoid unnecessary comments.
- **Security**: Never leak credentials or sensitive values in logs.

# apps/migrate/ — Database migration runner

One-shot binary. Runs `goose up` against the configured Postgres database using the SQL files in `backend/migrations/`.

## Usage

```bash
go run ./apps/migrate          # run all pending migrations
make migrate                   # same via Makefile
```

## Rules

- Always run before starting any other service against a fresh database
- Migration files live in `backend/migrations/` — never modify existing migration files, only add new ones
- Uses goose sequential numbering: `NNN_description.sql`

# chat-migration-cli

A one-shot CLI tool that migrates data from the legacy monolithic chat service database to the new microservices database. Both sources are PostgreSQL.

## How it works

Migration runs as an ordered sequence of steps. Each step is idempotent and resumable — progress is checkpointed after every page, so a failed or interrupted run can be restarted from where it left off.

### Steps

| Order | Step | What it does |
|-------|------|--------------|
| 1 | `clients_to_contacts` | Migrates external client users to contacts |
| 2 | `bots_to_contacts` | Migrates flow bots to contacts |
| 3 | `conversations` | Groups legacy conversations by `(initiator, flow)` and creates chat threads |
| 4 | `members` | Creates thread dialog members for all participants |
| 5 | `messages` | Migrates all messages, file attachments and interactive content |

Steps that have already completed are skipped automatically on re-runs. Use `MIGRATION_START_FROM_STEP` to resume from a specific step.

### Pagination

The conversations, members, and messages steps use keyset pagination ordered by `(initiator_id, flow_id)`. This avoids the O(N²) cost of OFFSET-based pagination on large datasets.

## Configuration

All options are read from environment variables prefixed with `MIGRATION_`.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MIGRATION_OLD_DB_DSN` | yes | — | Postgres DSN for the legacy chat database |
| `MIGRATION_NEW_DB_DSN` | yes | — | Postgres DSN for the new microservices database |
| `MIGRATION_OLD_DB_MAX_CONNS` | no | `5` | Connection pool size for the legacy DB |
| `MIGRATION_NEW_DB_MAX_CONNS` | no | `10` | Connection pool size for the new DB |
| `MIGRATION_START_FROM_STEP` | no | _(all)_ | Start from this step, skipping earlier ones |
| `MIGRATION_LOG_LEVEL` | no | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `MIGRATION_LOG_JSON` | no | `false` | Emit structured JSON logs instead of text |

DSN format: `postgres://user:password@host:5432/dbname?sslmode=disable`

## Usage

```sh
# Full migration
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
./chat-migration-cli

# Resume from a specific step
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
MIGRATION_START_FROM_STEP=messages \
./chat-migration-cli

# With debug logging
MIGRATION_LOG_LEVEL=debug \
MIGRATION_LOG_JSON=true \
...
```

## Building

```sh
go build -o chat-migration-cli .
```

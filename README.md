# chat-migration-cli

A CLI tool that migrates data from the legacy monolithic chat service database to the new microservices database. Both sources are PostgreSQL.

The tool supports two modes:

- **Migration mode** (default) — full one-shot migration of all historical data.
- **Sync mode** — incremental re-run that picks up only records created since the last completed migration step. Safe to run repeatedly without duplicating data.

## How it works

Migration runs as an ordered sequence of steps. Each step is idempotent and resumable — progress is checkpointed after every page, so a failed or interrupted run can be restarted from where it left off.

### Migration mode steps

| Order | Step | What it does |
|-------|------|--------------|
| 1 | `clients_to_contacts` | Migrates external client users to contacts |
| 1b | `portal_client_to_contact` | Migrates portal clients to contacts (only runs if `MIGRATION_MIGRATE_PORTAL_CLIENTS` is enabled) |
| 1c | `portal_apps_to_accounts` | Migrates portal service apps to accounts (only runs if `MIGRATION_MIGRATE_PORTAL_CLIENTS` is enabled). No sync-mode counterpart |
| 2 | `bots_to_contacts` | Migrates flow bots to contacts (or links to an existing contact if `MIGRATION_BOT_MAPPING_TABLE` maps the bot) |
| 3 | `conversations` | Groups legacy conversations by `(initiator, flow)` and creates chat threads |
| 4 | `members` | Creates thread dialog members for all participants |
| 5 | `messages` | Migrates all messages, file attachments and interactive content |
| 6 | `facebook_and_whatsapp` | Migrates Facebook and WhatsApp provider configs to gates and Meta apps. Makes outbound HTTP calls to the Meta Graph API to resolve WhatsApp Business Account phone numbers |
| 7 | `sync_contact_vias` | Syncs contact communication channels (vias) after all contacts and providers are in place |

Steps that have already completed are skipped automatically on re-runs. Use `MIGRATION_START_FROM_STEP` to resume from a specific step.

### Sync mode steps

Sync mode runs a parallel set of steps prefixed with `sync_mode_`. Each step queries the last completion timestamp of its counterpart migration step and fetches only records created after that point.

| Order | Step | What it does |
|-------|------|--------------|
| 1 | `sync_mode_clients_to_contacts` | Inserts new clients created since last run; existing records are skipped |
| 1b | `sync_mode_portal_client_to_contact` | Inserts new portal clients created since last run (only runs if `MIGRATION_MIGRATE_PORTAL_CLIENTS` is enabled) |
| 2 | `sync_mode_bots_to_contacts` | Inserts new bots created since last run; existing records are skipped (or links to an existing contact if `MIGRATION_BOT_MAPPING_TABLE` maps the bot) |
| 3 | `sync_mode_conversations` | Creates threads for new conversations; adds new conversation IDs to existing threads for the same `(initiator, flow)` pair |
| 4 | `sync_mode_members` | Adds full member set to newly created threads; adds only new internal users to existing threads |
| 5 | `sync_mode_messages` | Migrates messages from conversations created since last run |
| 6 | `sync_mode_facebook_and_whatsapp` | Migrates new Facebook and WhatsApp provider configs created since last run |
| 7 | `sync_mode_sync_contact_vias` | Re-syncs contact communication channels |

On the very first sync run (when no previous migration has completed), the timestamp falls back to the epoch, so all records are processed — equivalent to running a full migration.

### Pagination

Steps use one of three pagination strategies:

- **Two-value keyset pagination** (`conversations`, `members`, `messages`) ordered by `(initiator_id, flow_id)`. This avoids the O(N²) cost of OFFSET-based pagination on large datasets.
- **Single-value keyset pagination** (`clients_to_contacts`) — `WHERE c.id > $afterID ORDER BY c.id LIMIT $limit` on the legacy client's primary key, for the same reason as above.
- **Offset pagination** (`bots_to_contacts`, `facebook_and_whatsapp`) — plain `OFFSET`/`LIMIT` paging over a deterministically ordered query, so repeated runs resume against the same row order.

Every step commits its destination-DB writes and saves its checkpoint in the same transaction, once per page, so a crash or transient error at any point resumes from the last committed page rather than restarting the whole step.

## Configuration

All options are read from environment variables prefixed with `MIGRATION_`.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MIGRATION_OLD_DB_DSN` | yes | — | Postgres DSN for the legacy chat database |
| `MIGRATION_NEW_DB_DSN` | yes | — | Postgres DSN for the new microservices database |
| `MIGRATION_OLD_DB_MAX_CONNS` | no | `5` | Connection pool size for the legacy DB |
| `MIGRATION_NEW_DB_MAX_CONNS` | no | `10` | Connection pool size for the new DB |
| `MIGRATION_SYNC_MODE` | no | `false` | Run in sync mode instead of full migration mode |
| `MIGRATION_MIGRATE_PORTAL_CLIENTS` | no | `false` | Include portal clients in the migration (runs `portal_client_to_contact` and `portal_apps_to_accounts` steps) |
| `MIGRATION_START_FROM_STEP` | no | _(all)_ | Start from this step, skipping earlier ones |
| `MIGRATION_SINGLE_STEP` | no | `false` | Run only the step named by `MIGRATION_START_FROM_STEP`, then stop. Requires `MIGRATION_START_FROM_STEP` to be set. Resumes a not-yet-completed step from its last saved progress; fails if the step is already completed (outside sync mode - sync-mode steps remain re-runnable) |
| `MIGRATION_LOG_LEVEL` | no | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `MIGRATION_LOG_JSON` | no | `false` | Emit structured JSON logs instead of text |
| `MIGRATION_ENCRYPTION_KEY` | yes | — | 32-byte AES-256 key used to encrypt provider tokens at rest |
| `MIGRATION_BOT_MAPPING_TABLE` | no | (none) | Optional schema.table reference (e.g. public.bot_mapping) to a client-managed table mapping pre-existing old bots to already-created new bot contacts. See "Pre-existing bot mapping" below. |

DSN format: `postgres://user:password@host:5432/dbname?sslmode=disable`

`MIGRATION_ENCRYPTION_KEY` must be exactly 32 characters (256 bits). Tokens stored in the new database (Facebook page access tokens and WhatsApp Business access tokens) are encrypted with AES-256-GCM using this key.

### Pre-existing bot mapping

If you have bots already created in the new microservices database *before* running the migration, and you want those existing contacts to link to the old bot's chat history, use `MIGRATION_BOT_MAPPING_TABLE` to provide a mapping table.

**When to use:** Your client creates bot contacts out-of-band (not via this tool), and during migration you want old conversations involving those bots to attach to the existing contact instead of creating a fresh one.

**Who manages it:** You create and populate this table yourself *before* running migration. The tool never creates, seeds, or migrates the mapping table.

**Table shape:** A two-column table with no foreign keys required:
- `old_bot_id` (integer) — must equal the old database's `chat.bot.flow_id`
- `new_bot_id` (uuid) — must equal an existing new-database `im_contact.contact.id` row where `is_bot = true`

**Location:** The table must live in the *new* database (the same connection as `im_contact.contact`), so create it in the new microservices database, e.g.:

```sql
CREATE TABLE public.bot_mapping (
  old_bot_id INTEGER PRIMARY KEY,
  new_bot_id UUID NOT NULL
);
```

`old_bot_id` should be unique (a `PRIMARY KEY`/`UNIQUE` constraint, as above) — the tool does not enforce this, and if the table contains duplicate `old_bot_id` rows it logs a warning and keeps whichever row Postgres returns last (unspecified order). `new_bot_id` should be `NOT NULL` — a NULL value fails that row's scan and aborts the whole step.

Then set: `MIGRATION_BOT_MAPPING_TABLE=public.bot_mapping`

**When unset:** If `MIGRATION_BOT_MAPPING_TABLE` is not set, the tool creates a new contact for every bot — byte-identical behavior to before this feature existed.

**Validation:** The tool does *not* check that `new_bot_id` corresponds to an existing contact. A misconfigured mapping will surface as a failure in a later migration step (e.g., a foreign-key violation or an unresolved reference), not at the bots step itself. The `MIGRATION_BOT_MAPPING_TABLE` value's `schema.table` format *is* validated at startup (before any DB connection is made) — a malformed value (missing `.`, empty schema, or empty table) exits immediately with an error instead of silently running unmapped.

**Known limitations:**
- **`old_bot_id` is not domain-scoped.** The old database's `chat.bot` rows are keyed by `(flow_id, dc)`, so the same `flow_id` can in principle repeat across domains. The mapping table is keyed by bare `flow_id` only, so if your old DB has non-unique `flow_id` values across domains, bots in different domains that share a `flow_id` will all resolve to the same mapped contact. This is safe only if your deployment's `flow_id` values are globally unique.
- **Sync mode only sees new bots.** `sync_mode_bots_to_contacts` only processes bots created since the last run. If you add a mapping row *after* `bots_to_contacts` (or a prior sync run) has already created a contact for that bot, the bot won't be reprocessed and the mapping is never applied — populate the mapping table before the run that will first encounter each bot.
- **`records_migrated` undercounts mapped bots.** A mapped bot inserts a `chat_migration` row but no new contact, so it does not contribute to the step's reported `records_migrated` count. This is expected — not a sign of data loss — but means the count reflects contacts *created*, not bots *processed*.

## Usage

```sh
# Full migration
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
MIGRATION_ENCRYPTION_KEY="<32-character-key>" \
./chat-migration-cli

# Incremental sync (safe to run repeatedly)
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
MIGRATION_ENCRYPTION_KEY="<32-character-key>" \
MIGRATION_SYNC_MODE=true \
./chat-migration-cli

# Resume from a specific step
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
MIGRATION_ENCRYPTION_KEY="<32-character-key>" \
MIGRATION_START_FROM_STEP=messages \
./chat-migration-cli

# Run exactly one step, then stop (fails if the step is already completed outside sync mode)
MIGRATION_OLD_DB_DSN="postgres://..." \
MIGRATION_NEW_DB_DSN="postgres://..." \
MIGRATION_ENCRYPTION_KEY="<32-character-key>" \
MIGRATION_START_FROM_STEP=messages \
MIGRATION_SINGLE_STEP=true \
./chat-migration-cli

# With debug logging
MIGRATION_LOG_LEVEL=debug \
MIGRATION_LOG_JSON=true \
...
```

## Prerequisites

- Go 1.25+
- Network access to `https://graph.facebook.com` is required during the `facebook_and_whatsapp` step to resolve WhatsApp Business Account phone numbers from the Meta Graph API.

The tool auto-creates two tracking tables (`chat_migration` and `chat_migration_step`) in the new database on first run — no manual schema preparation is needed.

## Building

```sh
go build -o chat-migration-cli .
```

# CDC Subscriber Plugin Prompt

## Objective

Create `gopipe-cdc` plugin module for database CDC (Change Data Capture).

## Key Specs

- **PostgreSQL**: Logical Replication (pgoutput)
- **MySQL/MariaDB**: Binary Log (binlog)
- **Module**: `github.com/fxsml/gopipe-cdc` (separate repo)
- **Interface**: Implements `message.Receiver`

## Core Interface

```go
type CDCReceiver interface {
    message.Receiver
    Acknowledge(ctx context.Context, msg *message.Message) error
    Seek(ctx context.Context, offset string) error
    CurrentOffset() string
    io.Closer
}
```

## CloudEvents Output

- `type`: `db.{table}.{operation}` (e.g., `db.users.update`)
- `source`: `{prefix}/{schema}` (e.g., `postgres://mydb/public`)
- `subject`: `{table}/{pk}` (e.g., `users/123`)
- `data`: `{"before": {...}, "after": {...}}`

## Features

1. Table filtering by schema/name
2. Field filtering (exclude/always-include columns)
3. Offset persistence (file, SQL, memory)
4. Resume from WAL LSN / binlog position

## PR Sequence

1. Core types + interfaces
2. Offset storage
3. PostgreSQL implementation
4. MySQL implementation
5. SQL offset store
6. Examples + docs

## Dependencies

```
github.com/jackc/pglogrepl     # PostgreSQL
github.com/go-mysql-org/go-mysql # MySQL
```

## Reference

See full plan: [cdc-subscriber.md](cdc-subscriber.md)

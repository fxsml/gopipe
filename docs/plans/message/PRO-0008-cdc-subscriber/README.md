# PRO-0008: CDC Subscriber

**Date:** 2025-12-15
**Status:** Proposed
**Author:** Claude

## Executive Summary

Create `gopipe-cdc`, a separate Go module providing database Change Data Capture (CDC) functionality for PostgreSQL and MariaDB/MySQL. The plugin integrates with gopipe's `message.Receiver` interface and outputs CloudEvents-compliant messages.

## Vision

```
┌─────────────────────────────────────────────────────────────────┐
│                         Database                                 │
│  PostgreSQL (WAL/pgoutput)    MariaDB/MySQL (binlog)            │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      gopipe-cdc Plugin                           │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────────┐    │
│  │ CDC Receiver│ → │ Field Filter│ → │ CloudEvents Mapper  │    │
│  └─────────────┘   └─────────────┘   └─────────────────────┘    │
│                                              │                   │
│  ┌─────────────────────────────────────────┐ │                   │
│  │           Offset Store                   │◄┘                   │
│  │  (File / SQL / Memory)                   │                    │
│  └─────────────────────────────────────────┘                    │
└───────────────────────────────┬─────────────────────────────────┘
                                │ message.Receiver interface
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                        gopipe Core                               │
│  Subscriber → Router → Handler → Publisher                       │
└─────────────────────────────────────────────────────────────────┘
```

## Requirements

| Requirement | Decision |
|-------------|----------|
| PostgreSQL CDC | Logical Replication (pgoutput protocol) |
| MariaDB/MySQL CDC | Binary Log (binlog) replication |
| Package Location | Separate plugin: `github.com/fxsml/gopipe-cdc` |
| Table Filtering | Watch specific tables by schema/name |
| Field Filtering | Exclude columns, always-include columns |
| Offset Persistence | Resume from WAL LSN / binlog position |
| Output Format | CloudEvents-compliant messages |

## Package Structure

```
gopipe-cdc/
├── go.mod                          # github.com/fxsml/gopipe-cdc
├── go.sum
├── README.md
├── doc.go
│
├── cdc.go                          # CDCEvent, Operation enum
├── config.go                       # CDCConfig, TableFilter, FieldFilter
├── receiver.go                     # CDCReceiver interface
├── cloudevents.go                  # CloudEvents mapping utilities
├── filter.go                       # Field filtering logic
│
├── offset/
│   ├── offset.go                   # OffsetStore interface
│   ├── file.go                     # FileOffsetStore (JSON file)
│   ├── memory.go                   # MemoryOffsetStore (testing)
│   └── sql.go                      # SQLOffsetStore (generic SQL)
│
├── postgres/
│   ├── receiver.go                 # PostgresCDCReceiver
│   ├── config.go                   # PostgresConfig
│   ├── replication.go              # pglogrepl wrapper
│   ├── decoder.go                  # pgoutput protocol decoder
│   └── receiver_test.go
│
├── mysql/
│   ├── receiver.go                 # MySQLCDCReceiver
│   ├── config.go                   # MySQLConfig
│   ├── binlog.go                   # go-mysql binlog wrapper
│   ├── decoder.go                  # Row event decoder
│   └── receiver_test.go
│
├── internal/testutil/
│   ├── docker.go                   # Docker container helpers
│   └── fixtures.go                 # Test data generators
│
└── examples/
    ├── postgres-basic/main.go
    └── mysql-basic/main.go
```

## Core Types and Interfaces

### CDCEvent

```go
// cdc.go

type Operation string

const (
    OperationInsert Operation = "insert"
    OperationUpdate Operation = "update"
    OperationDelete Operation = "delete"
)

type CDCEvent struct {
    Database    string                 // Database name
    Schema      string                 // Schema (PostgreSQL) or empty
    Table       string                 // Table name
    Operation   Operation              // insert, update, delete
    Timestamp   time.Time              // When change occurred
    LSN         string                 // Replication position
    Transaction string                 // Transaction ID
    Before      map[string]any         // Previous row (update/delete)
    After       map[string]any         // New row (insert/update)
    PrimaryKey  []string               // PK column names
}
```

### CDCReceiver Interface

```go
// receiver.go

type CDCReceiver interface {
    message.Receiver

    // Acknowledge commits the offset after successful processing
    Acknowledge(ctx context.Context, msg *message.Message) error

    // Seek moves to a specific replication position
    Seek(ctx context.Context, offset string) error

    // CurrentOffset returns current replication position
    CurrentOffset() string

    io.Closer
}
```

### Configuration

```go
// config.go

type CDCConfig struct {
    DSN          string          // Database connection string
    Tables       []TableFilter   // Tables to capture (empty = all)
    FieldFilters []FieldFilter   // Per-table field filtering
    OffsetStore  OffsetStore     // Offset persistence (required)
    BatchSize    int             // Max events per Receive (default: 100)
    BatchTimeout time.Duration   // Max wait for batch (default: 100ms)
    SourcePrefix string          // CloudEvents source prefix
}

type TableFilter struct {
    Schema string   // Schema name (PostgreSQL only)
    Table  string   // Table name (supports * wildcard)
}

type FieldFilter struct {
    Schema  string   // Schema name
    Table   string   // Table name
    Exclude []string // Columns to exclude (e.g., ["password", "ssn"])
    Include []string // Always include (overrides Exclude)
}
```

### Offset Storage

```go
// offset/offset.go

type Offset struct {
    Position  string            // Database-specific offset
    Timestamp time.Time         // When stored
    Metadata  map[string]string // Optional context
}

type OffsetStore interface {
    Load(ctx context.Context, slotName string) (*Offset, error)
    Save(ctx context.Context, slotName string, offset *Offset) error
    Delete(ctx context.Context, slotName string) error
}
```

## CloudEvents Mapping

### Attribute Mapping

| CloudEvents | Source | Example |
|-------------|--------|---------|
| `id` | UUID v4 | `"550e8400-e29b-41d4-..."` |
| `source` | `{prefix}/{schema}` | `"postgres://mydb/public"` |
| `type` | `db.{table}.{operation}` | `"db.users.update"` |
| `specversion` | Always `"1.0"` | `"1.0"` |
| `subject` | `{table}/{pk_value}` | `"users/123"` |
| `time` | Event timestamp | `"2025-12-15T10:30:00Z"` |
| `datacontenttype` | Always JSON | `"application/json"` |

### Extension Attributes

| Extension | Purpose | Example |
|-----------|---------|---------|
| `cdcoperation` | Original operation | `"update"` |
| `cdclsn` | PostgreSQL LSN | `"0/16B3748"` |
| `cdcbinlog` | MySQL position | `"binlog.000001:12345"` |
| `cdctable` | Full table ref | `"public.users"` |
| `cdctransaction` | Transaction ID | `"12345"` |

### Data Payload Structure

```json
{
  "before": {
    "id": 123,
    "email": "old@example.com",
    "name": "Old Name"
  },
  "after": {
    "id": 123,
    "email": "new@example.com",
    "name": "New Name"
  }
}
```

## PostgreSQL Implementation

### Architecture

```
PostgreSQL Server
  WAL Log → Publication → pgoutput (decoder)
                              │
                              ▼ Logical Replication
PostgresCDCReceiver
  pglogrepl (stream) → Decoder → Filter → CloudEvents
                                              │
  OffsetStore ◄──── LSN tracking ─────────────┘
```

### Key Components

- **pglogrepl**: Handles logical replication protocol
- **pgoutput decoder**: Parses Relation, Insert, Update, Delete, Commit messages
- **Relation cache**: Stores table schema for row decoding

### PostgreSQL Config

```go
type PostgresConfig struct {
    cdc.CDCConfig
    SlotName          string // Replication slot name
    Publication       string // Publication name (must exist)
    CreatePublication bool   // Auto-create publication
}
```

## MariaDB/MySQL Implementation

### Architecture

```
MySQL/MariaDB Server
  Binlog (ROW format) → Row events
                            │
                            ▼ Binlog Replication Protocol
MySQLCDCReceiver
  go-mysql canal → Decoder → Filter → CloudEvents
                                          │
  OffsetStore ◄──── binlog pos ───────────┘
```

### Key Components

- **go-mysql canal**: Handles binlog streaming
- **Event handler**: Processes Insert/Update/Delete row events
- **Position tracking**: binlog file + position

### MySQL Config

```go
type MySQLConfig struct {
    cdc.CDCConfig
    ServerID uint32 // Unique server ID for replication
    Flavor   string // "mysql" or "mariadb" (auto-detect)
}
```

## Field Filtering

### Logic

```go
func (f *FieldFilterSet) Apply(schema, table string, row map[string]any) map[string]any {
    filter := f.filters[schema+"."+table]
    if filter == nil {
        return row
    }

    result := make(map[string]any)
    for col, val := range row {
        // Always-include overrides exclude
        if filter.include[col] {
            result[col] = val
            continue
        }
        // Skip excluded columns
        if filter.exclude[col] {
            continue
        }
        result[col] = val
    }
    return result
}
```

### Example Config

```go
FieldFilters: []cdc.FieldFilter{
    {
        Table:   "users",
        Exclude: []string{"password_hash", "ssn", "credit_card"},
        Include: []string{"id", "email"}, // Always include for correlation
    },
}
```

## Dependencies

```go
// go.mod
module github.com/fxsml/gopipe-cdc

go 1.24

require (
    github.com/fxsml/gopipe v0.x.x           // Core (Receiver, Message)
    github.com/jackc/pgx/v5 v5.x.x           // PostgreSQL driver
    github.com/jackc/pglogrepl v0.x.x        // PostgreSQL logical replication
    github.com/go-mysql-org/go-mysql v1.x.x  // MySQL binlog replication
)
```

## Testing Strategy

### Unit Tests

- Decoder tests with captured binary data
- Field filter include/exclude logic
- CloudEvents mapping correctness
- Offset store persistence/recovery

### Integration Tests (Docker)

```go
// PostgreSQL container with:
// wal_level = logical, max_replication_slots = 4

// MySQL container with:
// binlog_format = ROW, binlog_row_image = FULL
```

### Test Scenarios

1. Basic CDC flow: Insert/Update/Delete → verify events
2. Offset persistence: Stop/restart → no data loss
3. Table filtering: Only configured tables captured
4. Field filtering: Sensitive columns excluded
5. Transaction handling: Multiple changes in single tx

## PR Sequence

| PR | Content | Deliverables |
|----|---------|--------------|
| **1** | Core types + interfaces | `cdc.go`, `config.go`, `receiver.go`, `cloudevents.go`, `filter.go` |
| **2** | Offset storage | `offset/offset.go`, `offset/file.go`, `offset/memory.go` |
| **3** | PostgreSQL implementation | `postgres/*`, integration tests |
| **4** | MySQL implementation | `mysql/*`, integration tests |
| **5** | SQL offset store | `offset/sql.go` |
| **6** | Examples + docs | `examples/`, `README.md`, ADR |

## Usage Example

```go
package main

import (
    "context"
    "log"

    "github.com/fxsml/gopipe-cdc"
    "github.com/fxsml/gopipe-cdc/offset"
    "github.com/fxsml/gopipe-cdc/postgres"
    "github.com/fxsml/gopipe/message"
)

func main() {
    ctx := context.Background()

    // Create CDC receiver
    receiver, err := postgres.NewPostgresCDCReceiver(ctx, postgres.PostgresConfig{
        CDCConfig: cdc.CDCConfig{
            DSN: "postgres://user:pass@localhost/mydb",
            Tables: []cdc.TableFilter{
                {Schema: "public", Table: "users"},
                {Schema: "public", Table: "orders"},
            },
            FieldFilters: []cdc.FieldFilter{
                {
                    Table:   "users",
                    Exclude: []string{"password_hash"},
                    Include: []string{"id", "email"},
                },
            },
            OffsetStore:  offset.NewFileOffsetStore("offsets.json"),
            SourcePrefix: "postgres://mydb",
        },
        Publication: "gopipe_pub",
        SlotName:    "gopipe_slot",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer receiver.Close()

    // Wrap with gopipe Subscriber
    subscriber := message.NewSubscriber(receiver, message.SubscriberConfig{})
    events := subscriber.Subscribe(ctx, "")

    // Process CDC events
    for msg := range events {
        eventType, _ := msg.Attributes.Type()
        subject, _ := msg.Attributes.Subject()
        log.Printf("CDC: type=%s subject=%s", eventType, subject)

        // Acknowledge to commit offset
        receiver.Acknowledge(ctx, msg)
    }
}
```

## Reference Files

| File | Purpose |
|------|---------|
| `message/message.go` | `Receiver` interface to implement |
| `message/attributes.go` | CloudEvents attribute constants |
| `message/subscriber.go` | `Subscriber` wrapper for polling |
| `message/broker/io.go` | Reference `Receiver` implementation |
| `message/uuid.go` | UUID generation for event IDs |

## Related Documents

- [ADR 0028: Subscriber Patterns](../adr/0028-generator-source-patterns.md) - BrokerSubscriber interface
- [ADR 0025: SQL Event Store](../adr/0025-sql-event-store.md) - Persistence patterns
- [Architecture Roadmap](architecture-roadmap.md) - Layer integration
- [CloudEvents State](../state/cloudevents-state.md) - Current implementation status

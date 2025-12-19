# PRO-0001: goengine Repository Initialization

**Status:** Proposed
**Priority:** High (Prerequisite for all goengine work)

## Overview

goengine will be a **separate repository** (`fxsml/goengine`) focused on CloudEvents messaging infrastructure. This plan covers the initial repository setup.

## Goals

1. Create new `fxsml/goengine` repository
2. Set up project structure matching gopipe conventions
3. Establish CI/CD pipeline
4. Create initial documentation structure
5. Define dependency on gopipe

## Repository Structure

```
goengine/
├── .github/
│   └── workflows/
│       └── ci.yml
├── adapters/           # Broker adapters (NATS, Kafka, RabbitMQ)
│   ├── nats/
│   ├── kafka/
│   └── rabbitmq/
├── cqrs/               # CQRS handlers (from gopipe/message/cqrs)
├── engine/             # Engine orchestration
├── message/            # CloudEvents message types
├── router/             # Message routing
├── docs/
│   ├── adr/            # ADRs starting at 0001
│   ├── manual/
│   ├── plans/
│   └── README.md
├── examples/
├── go.mod
├── go.sum
├── CHANGELOG.md
├── CLAUDE.md
├── CONTRIBUTING.md
├── LICENSE
├── Makefile
└── README.md
```

## Tasks

### Task 1: Create Repository

```bash
# On GitHub
gh repo create fxsml/goengine --public --description "CloudEvents messaging engine for Go"

# Clone and initialize
git clone https://github.com/fxsml/goengine.git
cd goengine
```

### Task 2: Initialize Go Module

```go
// go.mod
module github.com/fxsml/goengine

go 1.21

require (
    github.com/fxsml/gopipe v0.11.0  // After gopipe cleanup
)
```

### Task 3: Copy Documentation

Copy from `gopipe/docs/goengine/`:
- ADRs (already numbered 0001-0011+)
- Plans (renumber from 0001)
- Features that become manual content

### Task 4: Set Up CI

Copy and adapt from gopipe:
- `.github/workflows/ci.yml`
- `Makefile`

### Task 5: Create Initial Code Structure

```bash
mkdir -p adapters/{nats,kafka,rabbitmq}
mkdir -p cqrs engine message router
mkdir -p docs/{adr,manual,plans}
mkdir -p examples
```

## Dependencies

- gopipe v0.11.0+ with:
  - ProcessorConfig (PRO-0026)
  - Subscriber[T] (PRO-0028)
  - Sender/Receiver removed (PRO-0030)

## Acceptance Criteria

- [ ] Repository created on GitHub
- [ ] Go module initialized with gopipe dependency
- [ ] CI pipeline passing
- [ ] Documentation structure in place
- [ ] README with project overview
- [ ] CHANGELOG initialized
- [ ] CONTRIBUTING.md copied and adapted

## Related

- [PRO-0002-migration](../PRO-0002-migration/) - Migration from gopipe
- [gopipe PRO-0030](../../../adr/PRO-0030-remove-sender-receiver.md) - Sender/Receiver removal

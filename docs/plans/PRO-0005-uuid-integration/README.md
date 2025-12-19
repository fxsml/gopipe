# PRO-0005: UUID Integration

**Status:** Won't Do (for gopipe)
**Priority:** N/A
**Related ADRs:** -

## Decision

**This plan will NOT be implemented in gopipe.**

The `message` package and all related components are being migrated to **goengine** (see [goengine plans](../../goengine/plans/)). UUID integration for message IDs will be handled in goengine as part of CloudEvents compliance.

## Cleanup Required

The partial implementation in the `develop` branch should be removed:

**TODO:** Remove from `develop` branch:
- `message/uuid.go` - Remove or migrate to goengine
- Any UUID auto-generation in `message/message.go`

## What Stays in gopipe

gopipe will focus on:
- Channel operations (`channel/`)
- Pipeline primitives (`Processor`, `Pipe`, `Subscriber`)
- Generic middleware

The `TypedMessage[T]` type in gopipe does NOT require CloudEvents attributes or auto-generated UUIDs.

## goengine Responsibility

UUID/ID generation will be handled by goengine using `github.com/google/uuid`:
- CloudEvents requires `id` attribute (RFC 4122 UUID recommended)
- goengine's `Message` will auto-generate IDs on creation
- External dependencies are allowed in goengine (unlike gopipe core)

**goengine Documentation:**
- [PRO-0011: UUID Integration Plan](../../goengine/plans/PRO-0011-uuid-integration/)
- [PRO-0012: Google UUID ADR](../../goengine/adr/PRO-0012-google-uuid.md)

## Related

- [goengine Plans](../../goengine/plans/) - Where message functionality moves
- [gopipe PRO-0006: Package Restructuring](../PRO-0006-package-restructuring/) - Cleanup plan

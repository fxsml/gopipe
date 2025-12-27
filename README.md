# gopipe

[![CI](https://github.com/fxsml/gopipe/actions/workflows/ci.yml/badge.svg)](https://github.com/fxsml/gopipe/actions/workflows/ci.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/fxsml/gopipe.svg)](https://pkg.go.dev/github.com/fxsml/gopipe)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A lightweight, generic Go library for building composable data pipelines with zero dependencies.

## Packages

| Package | Description |
|---------|-------------|
| `channel` | Stateless channel operations (Filter, Transform, Merge, Route, etc.) |
| `pipe` | Stateful components with lifecycle (Merger, Distributor, middleware) |
| `message` | CloudEvents message handling with type-based routing |

### channel vs pipe

| Need | Use | Why |
|------|-----|-----|
| Simple transform/filter | `channel` | Stateless, functional |
| Merge fixed channels | `channel.Merge` | No dynamic adding |
| Add inputs at runtime | `pipe.Merger` | Concurrent-safe, configurable |
| Route by matcher | `pipe.Distributor` | Dynamic outputs, first-match |

## Installation

```bash
go get github.com/fxsml/gopipe
```

## Examples

See [examples/](examples/) for working examples.

## License

MIT

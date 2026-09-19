# Structure

## What lives where

The source commit has 12 tracked files. The executable lives under `cmd/pgdoor`, the working transport and future trace loop under `internal/proxy`, and the incomplete wire codec plus its tests under `internal/pgwire`. Root documentation records the learning plan, and `docs/benchmarks.md` is an unfilled measurement plan. There were no existing project-guide files, CI workflows, or deployment manifests.

```text
.
├── README.md, PLAN.md            current phase and future design
├── go.mod, Makefile             module and local tooling
├── .gitignore                   generated/local files
├── cmd/pgdoor/main.go           process and connection lifecycle
├── internal/proxy/relay.go      raw relay and trace scaffold
├── internal/pgwire/
│   ├── message.go               framing API and buffer wrappers
│   ├── startup.go               startup/cancel types and TODOs
│   ├── tags.go                  protocol constants and naming TODO
│   └── pgwire_test.go           failing codec specifications and fuzz target
└── docs/benchmarks.md           intended benchmark matrix; no results
```

## Root

| File | Responsibility | Key exports or targets | Called by / read by |
| --- | --- | --- | --- |
| [go.mod](../../go.mod#L1) | Module identity and Go minimum; no third-party requirements | `github.com/rushikeshg25/pgdoor`, `go 1.24` | Go toolchain |
| [Makefile](../../Makefile#L1) | Local build, run, trace, test, vet, fuzz, and clean commands | `build`, `run`, `trace`, `test`, `vet`, `fuzz`, `clean` | Developers using `make` |
| [README.md](../../README.md#L20) | Status table, phase-0 demo, dependency policy | None | New contributors |
| [PLAN.md](../../PLAN.md#L219) | Future architecture, protocol primer, phased learning and decisions | None | Contributors; not proof of implemented features |

## Command

[cmd/pgdoor](../../cmd/pgdoor) is `package main`. It owns connection lifetime; the proxy package receives already-open connections.

| File | Responsibility | Key symbols | Called by |
| --- | --- | --- | --- |
| [main.go](../../cmd/pgdoor/main.go#L29) | Flags, stderr logger, listener, signals, accept loop, upstream dialing, session dispatch | `main`, private `handle` | OS executable entry; accept-loop goroutines |

## Proxy

[internal/proxy](../../internal/proxy) contains one file with two transport strategies. The raw strategy works; the traced strategy's downstream structure exists but is blocked by unimplemented codec methods.

| File | Responsibility | Key exports or internal symbols | Called by |
| --- | --- | --- | --- |
| [relay.go](../../internal/proxy/relay.go#L34) | Bidirectional `io.Copy`, synchronized first error, half-closes; trace startup/message loop scaffold | `Relay`, `RelayTraced`; private `halfCloser` | Command's `handle`; `RelayTraced` would also call `Relay` on encryption requests |

## Wire codec

[internal/pgwire](../../internal/pgwire) supplies protocol vocabulary and tests for the next learning phase. Exported Go identifiers are still inside an `internal` package and are not a general public SDK.

| File | Responsibility | Key symbols and status | Called by |
| --- | --- | --- | --- |
| [message.go](../../internal/pgwire/message.go#L26) | Request/version constants, framing, direction, buffer wrappers | `Direction`, `RawMessage`, `Reader`, `Writer`, `MaxMessageSize`; `NewReader`, `NewWriter`, `Flush` work; other methods panic | Traced relay and codec tests |
| [startup.go](../../internal/pgwire/startup.go#L17) | Untagged startup/request types and intended serialization | `Startup`, `CancelRequest`, `StartupKind`, `FirstPacket`; `User`, `Database`, `ReadFirstPacket`, `AppendStartup`, `AppendCancelRequest` all panic | Traced relay and codec tests; append helpers reserved for later phases |
| [tags.go](../../internal/pgwire/tags.go#L17) | Frontend/backend tags, auth codes, transaction status bytes | `FTag*`, `BTag*`, `Auth*`, `Tx*` constants; `TagName` panics | Codec tests and future descriptions/pooling logic |
| [pgwire_test.go](../../internal/pgwire/pgwire_test.go#L16) | Hand-assembled fixtures, framing/validation/encoding/naming/startup checks, fuzz seed properties | `catchTODO`, `TestReadMessage_*`, `TestRoundTrip`, `TestAppendTo_ReusesBuffer`, `TestTagName_*`, `TestDescribe_IsReadableAndSafe`, `TestReadFirstPacket_*`, `TestStartup_DatabaseDefaultsToUser`, `FuzzReadMessage` | `go test`, `make fuzz` |

The tests are package-local and deliberately catch panic stubs rather than terminating on the first TODO. There are no tracked tests for the command or relay at this commit.

## Docs

| File | Responsibility | Key exports | Read by |
| --- | --- | --- | --- |
| [benchmarks.md](../benchmarks.md#L1) | Planned direct/session/transaction comparison, historical environment, explicit absence of results | None | Future benchmark work |

## Excluded

[.gitignore](../../.gitignore#L1) contains generated binary/test-output patterns and a local credential-file pattern; it is omitted from the design tables. Git internals, generated executables, and local runtime data are not source. No vendored dependencies or lockfiles are tracked. This newly added guide is documentation only.

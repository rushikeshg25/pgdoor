# pgdoor Project Guide

> Generated: 2026-09-20 from commit `12b8862b1d3255ce10ea95acb7a38f79c0c019ee`.

## What this is

pgdoor is a learning project aimed at a PostgreSQL connection pooler, but the current executable implements a transparent TCP relay with one fresh upstream connection per client ([main.go:79](../../cmd/pgdoor/main.go#L79)). Developers can point a PostgreSQL client at the relay to exercise connection handling while authentication and database state remain at the upstream server ([relay.go:34](../../internal/proxy/relay.go#L34)). The wire-protocol codec and traced relay are scaffolds, and session/transaction pooling, independent authentication, cancellation translation, and prepared-statement emulation remain roadmap work ([startup.go:94](../../internal/pgwire/startup.go#L94), [PLAN.md:302](../../PLAN.md#L302)).

## The five-file tour

| # | File | Why this one | Then look at |
| --- | --- | --- | --- |
| 1 | [cmd/pgdoor/main.go](../../cmd/pgdoor/main.go#L29) | Flags, listening, one handler per client, and upstream dialing | [Startup](02-flow.md#startup) |
| 2 | [internal/proxy/relay.go](../../internal/proxy/relay.go#L34) | The working byte relay and its half-close behavior | [Session lifecycle](02-flow.md#session-lifecycle) |
| 3 | [internal/pgwire/startup.go](../../internal/pgwire/startup.go#L94) | The first unimplemented operation reached by trace mode | [Trace mode](02-flow.md#trace-mode) |
| 4 | [internal/pgwire/message.go](../../internal/pgwire/message.go#L73) | Planned framing API, real buffer wrappers, and TODO boundaries | [Data model](01-architecture.md#data-model) |
| 5 | [internal/pgwire/pgwire_test.go](../../internal/pgwire/pgwire_test.go#L74) | Executable expectations for completing the codec | [Build and tests](02-flow.md#build-and-tests) |

## Run it

Use Go 1.24 or newer ([go.mod:3](../../go.mod#L3)). For a database demo, provide a running PostgreSQL server and `psql`; the repository does not launch or install them. The documented historical environment is PostgreSQL 17.6 and Go 1.24.6, not a version pin ([docs/benchmarks.md:15](../benchmarks.md#L15)).

From the repository root:

```bash
make build
./bin/pgdoor -listen 127.0.0.1:6432 -upstream localhost:5432
```

In another shell, with credentials accepted by that PostgreSQL server:

```bash
psql -h 127.0.0.1 -p 6432 -d postgres -c 'select current_user'
```

The explicit loopback address above differs from the default `:6432`, which does not restrict listening to loopback. Configuration is through four flags, with no application config file or environment loader in [main.go:30](../../cmd/pgdoor/main.go#L30):

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `:6432` | Local TCP listener |
| `-upstream` | `localhost:5432` | Upstream TCP endpoint |
| `-dial-timeout` | `5s` | Time limit for establishing each upstream connection |
| `-trace` | `false` | Select the incomplete message-aware path |

**Keep trace disabled for the working relay.** After a successful upstream dial, trace calls `ReadFirstPacket`, which currently panics without reading a packet; there is no recovery in the connection handler ([relay.go:77](../../internal/proxy/relay.go#L77), [startup.go:94](../../internal/pgwire/startup.go#L94), [main.go:67](../../cmd/pgdoor/main.go#L67)).

```bash
make vet
make test
```

`make test` is expected to fail at the codec TODOs. This is intentional scaffold state, not a green baseline; [PLAN.md:381](../../PLAN.md#L381) records the learning workflow.

## Verification of this guide

On 2026-09-20 with Go 1.24.6 on Darwin/arm64, `go vet ./...` and building `./cmd/pgdoor` passed. `go test ./...` failed at the existing phase-1 panic stubs, including fuzz seeds. A temporary external loopback harness verified exact bytes in both directions, a client write-half-close followed by the upstream response, and clean SIGTERM exit after the session ended. This did not test a real PostgreSQL server, pooling, or working trace mode. Guide links/source-line anchors were checked and Mermaid diagrams parsed with Mermaid 11.12.0.

## Reading order for this guide

1. [Architecture](01-architecture.md) — current components and the scaffold boundary.
2. [Flow](02-flow.md) — startup, relay, shutdown, trace, and tests.
3. [Structure](03-structure.md) — all significant source and support files.
4. [Tech stack](04-tech-stack.md) — actual dependencies, flags, and tooling.
5. [Decisions](05-decisions.md) — stated rationale, tradeoffs, and gotchas.

## Open questions

- What ownership rule should `ReadMessage` use for returned bodies? The scaffold explicitly leaves copying versus buffer reuse undecided ([message.go:124](../../internal/pgwire/message.go#L124)).
- How will startup parsing preserve buffered bytes when trace falls back to the raw socket? `NewReader` wraps a buffered reader, but the fallback passes the underlying connection to `Relay`, and the no-overread test checks bytes remaining in the underlying reader ([message.go:120](../../internal/pgwire/message.go#L120), [relay.go:112](../../internal/proxy/relay.go#L112), [pgwire_test.go:332](../../internal/pgwire/pgwire_test.go#L332)).
- What bounded drain/error policy should replace waiting indefinitely for the other relay direction or client session? Both relay and process shutdown wait without a deadline ([relay.go:66](../../internal/proxy/relay.go#L66), [main.go:73](../../cmd/pgdoor/main.go#L73)).
- What should traces redact before becoming operational tooling? The unreachable cancellation branch currently formats the cancellation secret, while message descriptions are intended to include query text ([relay.go:89](../../internal/proxy/relay.go#L89), [message.go:95](../../internal/pgwire/message.go#L95)). No actual secrets are reproduced in this guide.

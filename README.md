# pgdoor

A PostgreSQL connection pooler, written from scratch in Go with **zero dependencies**.

Postgres forks an OS process per connection, so throughput peaks somewhere around
`2–4 × cores` active connections and *declines* after that. Applications overshoot by
simple arithmetic — 40 pods × a pool of 25 is 1000 connections — while each of those
connections sits idle almost all the time. pgdoor multiplexes many client connections
onto a small number of server connections, borrowing one per *transaction*.

The interesting part is that a Postgres connection is **stateful**: open transactions,
session GUCs, prepared statements, `LISTEN`, advisory locks, temp tables. Handing one to
a different client is only safe if none of that leaks. Every design decision in this
repository is an answer to *which of those do I forbid, which do I emulate, and which do
I detect and refuse to pool?*

See **[PLAN.md](PLAN.md)** for the full write-up: why poolers exist, a protocol primer,
the architecture, and the phase-by-phase build.

## Status

| Phase | | |
|---|---|---|
| 0 | dumb TCP relay | ✅ working |
| 1 | wire codec + `--trace` | 🚧 scaffolded, tests red |
| 2 | own both handshakes (SCRAM) | ⬜ |
| 3 | session pooling | ⬜ |
| 4 | transaction pooling | ⬜ |
| 5 | query cancellation | ⬜ |
| 6 | prepared statement emulation | ⬜ |
| — | benchmarks | ⬜ |

## Try phase 0

```bash
make build
./bin/pgdoor -listen :6432 -upstream localhost:5432

# in another shell — everything works, because pgdoor isn't doing anything yet
psql -h localhost -p 6432 -d postgres -c 'select current_user'
```

Once phase 1 lands, `-trace` decodes every message while forwarding it unchanged:

```
C->S  Query           "SELECT 1"
S->C  RowDescription  1 field: ?column? oid=23
S->C  DataRow         ["1"]
S->C  CommandComplete "SELECT 1"
S->C  ReadyForQuery   I
```

## Layout

```
cmd/pgdoor/        entrypoint, flags, accept loop
internal/pgwire/   the v3 wire protocol — framing, tags, startup    ← phase 1 lives here
internal/proxy/    relay: byte-for-byte (phase 0) and traced (phase 1)
PLAN.md            why / what / how, in detail
```

## Zero dependencies

`go.mod` has no `require` block and should never grow one. Go 1.24 moved `crypto/pbkdf2`
and `crypto/hkdf` into the standard library, so even SCRAM-SHA-256 is reachable without
`x/crypto`. Adding a dependency here is a design failure, not a shortcut.

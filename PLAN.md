# pgdoor — a PostgreSQL connection pooler, built from scratch in Go

> A learning-first build plan. Every phase ends with something you can run against a real
> Postgres and a specific thing you now understand that you didn't before.

---

## Part 1 — WHY: the problem that makes poolers exist

### 1.1 Postgres is process-per-connection

When a client connects, the `postmaster` **forks a new OS process** (a "backend") that lives for
the entire connection. This is not a thread, not a goroutine — a full process.

Consequences:

| Cost | Detail |
|---|---|
| Memory | Each backend has its own catalog/relcache, plan cache, and `work_mem` allocations. Baseline is a few MB and grows with the queries it has run. 1000 idle connections is real RAM. |
| Fork latency | Connection setup = TCP + optional TLS + startup packet + SCRAM round trips + `fork()` + cache warm. Tens of ms. Paid on *every* new connection. |
| Scheduler pressure | 1000 backends mostly idle still means 1000 processes for the kernel to manage. |
| Shared-state contention | Snapshot acquisition, the lock manager, and `pg_stat_activity` all walk per-backend shared structures. PG14 substantially improved snapshot scalability, but the cost never reaches zero. |

The practical result: throughput on most hardware **peaks somewhere near `2–4 × cores` active
connections and then declines**. Past the peak, adding clients makes everything slower — a
textbook congestion collapse curve. `max_connections = 1000` doesn't buy you 1000 workers, it
buys you a way to fall off the cliff.

### 1.2 Why applications overshoot anyway

Nobody *decides* to open 1000 connections. It's arithmetic:

```
40 app pods  ×  pool size 25              = 1000 connections
+ 3 background workers × 10               =   30
+ a cron box, a migration runner, an analyst with a psql window
```

Each app instance sizes its pool locally with no idea the others exist. Serverless makes it
pathological — one connection per concurrent invocation, no reuse across cold starts.

### 1.3 The fix: multiplexing

Most connections are **idle almost all the time**. A web request holds a connection for ~2 ms of
query and ~200 ms of everything else. So put a proxy in the middle:

```
1000 clients ──▶  pgdoor  ──▶  25 server connections  ──▶  Postgres
   (mostly idle)     (multiplexes)      (busy)
```

A server connection is only borrowed for the *duration of a transaction*, then handed to whoever
is waiting. Clients see a connection that is always available; Postgres sees a small, steady,
efficiently-used set of backends.

### 1.4 Why this is hard — the whole intellectual content of the project

**A Postgres connection is stateful.** Handing a server connection to a different client is only
safe if nothing observable leaks between them. State that lives on a connection:

- the open transaction (obvious)
- session GUCs — `SET search_path`, `SET timezone`, `SET statement_timeout`, `SET ROLE`
- **prepared statements** — both protocol-level named statements and SQL `PREPARE`
- cursors/portals, especially `WITH HOLD`
- `LISTEN` registrations
- session-level advisory locks (`pg_advisory_lock`)
- temp tables
- `currval()` / `lastval()` sequence state
- deferred constraint state, `SET CONSTRAINTS`

Every pooler is a set of answers to: *which of these do I forbid, which do I emulate, and which do
I detect and refuse to pool?* That's the design space. That's what you're here to learn.

### 1.5 The three pooling modes

| Mode | Server conn is held for | Multiplexing win | Breaks |
|---|---|---|---|
| **session** | the whole client connection | ~none (only saves connect cost) | nothing |
| **transaction** | one transaction | **large** — the reason poolers exist | prepared statements, `LISTEN`, session advisory locks, `WITH HOLD` cursors, `SET` outside a txn, temp tables |
| **statement** | one statement | largest | multi-statement transactions entirely |

Transaction mode is the target. Everything interesting is in making its breakages survivable.

### 1.6 Prior art you're deliberately re-deriving

- **pgbouncer** — C, single-threaded libevent. The reference. Deliberately minimal.
- **pgcat** — Rust, adds load balancing / sharding / failover.
- **odyssey** — C, multi-threaded, Yandex.
- **supavisor** — Elixir, multi-tenant/cloud-shaped.

Go's goroutine-per-connection model gives you pgbouncer's behaviour with a fraction of the state
machine complexity — the concurrency *is* the runtime. That's the angle worth writing up.

---

## Part 2 — WHAT: the things you will actually understand at the end

1. **The Postgres v3 wire protocol**, by hand — framing, both query protocols, COPY, errors.
2. **SCRAM-SHA-256** end to end, and specifically *why a proxy cannot just forward auth*.
3. **Transaction boundary detection** from the wire — the `ReadyForQuery` status byte.
4. **Connection state hygiene** — what dirties a connection and how to detect or reset it.
5. **Out-of-band query cancellation** — a whole second TCP connection and a key-mapping problem.
6. **Prepared statement emulation** — rewriting client statement names onto shared server conns.
7. **Backpressure and fairness** — bounded pools, wait queues, timeouts, no goroutine leaks.
8. **Measuring it honestly** — `pgbench` curves, direct vs pooled, where the crossover is.

---

## Part 3 — Protocol primer (the notes you'll want open while coding)

### 3.1 Framing

Every message after the handshake is:

```
┌────────┬────────────────┬──────────────────────┐
│ type   │ length (int32) │ body                 │
│ 1 byte │ includes self, │ length - 4 bytes     │
│        │ excludes type  │                      │
└────────┴────────────────┴──────────────────────┘
```

**The startup packet is the exception: no type byte.** It is `length + payload`, where the first
int32 of the payload is a version code:

| Code | Meaning |
|---|---|
| `196608` (0x00030000) | protocol 3.0 — a real startup packet |
| `80877103` | SSLRequest — reply with a single byte `S` or `N`, *then* TLS handshake |
| `80877102` | CancelRequest — arrives on its own fresh connection, carries pid + secret |
| `80877104` | GSSENCRequest — reply `N` unless you support it |

### 3.2 Message tags worth memorising

**Frontend → backend:** `Q` Query · `P` Parse · `B` Bind · `D` Describe · `E` Execute · `S` Sync ·
`C` Close · `H` Flush · `X` Terminate · `d` CopyData · `c` CopyDone · `f` CopyFail

**Backend → frontend:** `R` Authentication · `S` ParameterStatus · `K` BackendKeyData ·
`Z` ReadyForQuery · `T` RowDescription · `D` DataRow · `C` CommandComplete · `E` ErrorResponse ·
`N` NoticeResponse · `1` ParseComplete · `2` BindComplete · `3` CloseComplete · `n` NoData ·
`t` ParameterDescription · `s` PortalSuspended · `A` NotificationResponse · `I` EmptyQueryResponse ·
`G` CopyInResponse · `H` CopyOutResponse · `W` CopyBothResponse

> **Trap #1:** tags are ambiguous across directions. `D` is Describe *and* DataRow. `C` is Close
> *and* CommandComplete. `E` is Execute *and* ErrorResponse. `S` is Sync *and* ParameterStatus.
> `H` is Flush *and* CopyOutResponse. Your decoder must be direction-aware — this is a design
> constraint on the codec API, not a detail.

### 3.3 Handshake

```
client                          pgdoor                         postgres
  │ ── SSLRequest ──────────────▶ │
  │ ◀───────────── 'S' or 'N' ─── │
  │ ══ TLS ══════════════════════ │
  │ ── Startup{user,database} ──▶ │
  │ ◀── Authentication SASL ───── │   (pgdoor terminates auth itself)
  │ ── SASLInitialResponse ─────▶ │
  │ ◀── SASLContinue ──────────── │
  │ ── SASLResponse ────────────▶ │
  │ ◀── SASLFinal, AuthOk ─────── │
  │ ◀── ParameterStatus × N ───── │
  │ ◀── BackendKeyData (FAKE) ─── │   ← pgdoor's own pid/secret, not the server's
  │ ◀── ReadyForQuery 'I' ─────── │
                                   │ ── (lazily, on first txn) ──▶ real server conn
```

### 3.4 The single most important byte in the project

`ReadyForQuery` carries a one-byte transaction status:

| Byte | Meaning | Pooler action (transaction mode) |
|---|---|---|
| `I` | idle, no transaction | **release the server connection to the pool** |
| `T` | inside a transaction block | hold it |
| `E` | failed transaction, awaiting ROLLBACK | hold it |

Transaction pooling, in one sentence: *when you relay a `Z` with status `I`, and the session isn't
dirty, give the server connection back.*

### 3.5 Why auth cannot be proxied

SCRAM-SHA-256 is challenge–response with a per-exchange nonce, and the client proof is bound to
the full `AuthMessage`. A man-in-the-middle that doesn't know the password **cannot** relay the
client's response to the server — the nonces differ. So a pooler must:

1. authenticate the client itself, against credentials it holds; **and**
2. authenticate to the server itself, as a real client.

Which means pgdoor needs verifiable credentials. Options, in increasing order of realism:

- a static `userlist.txt` of `user → password` (simplest, what you'll start with)
- store SCRAM verifiers, not passwords: `SCRAM-SHA-256$<iter>:<salt>$<StoredKey>:<ServerKey>` —
  lets you *verify* a client without knowing the plaintext, but then you can't originate a client
  SCRAM exchange to the server, so upstream needs its own credential
- pgbouncer's `auth_query`: hold one privileged connection and
  `SELECT usename, passwd FROM pg_shadow WHERE usename = $1` on demand

The cryptography you'll implement: `SaltedPassword = PBKDF2-HMAC-SHA256(password, salt, i)`,
`ClientKey = HMAC(SaltedPassword, "Client Key")`, `StoredKey = SHA256(ClientKey)`,
`ClientSignature = HMAC(StoredKey, AuthMessage)`, `ClientProof = ClientKey XOR ClientSignature`.
All of it is in `crypto/*` + `golang.org/x/crypto/pbkdf2`.

### 3.6 Query cancellation — trap #2

`psql`'s Ctrl-C does **not** send a message on the existing connection. It opens a *brand new TCP
connection* and sends a `CancelRequest` with the pid+secret from `BackendKeyData`.

If you naively forward the server's real `BackendKeyData` to clients, then in transaction pooling
a client's Ctrl-C will cancel **whatever query is running on that server connection right now** —
which may belong to a completely different tenant. A genuine cross-user data-integrity bug.

The fix: issue each client a *synthetic* key, keep a `map[fakeKey] → *ClientSession`, and on a
CancelRequest look up which server connection that client currently holds (if any) and forward a
cancel carrying the *real* key on a fresh connection to Postgres.

---

## Part 4 — Architecture

```
                    ┌──────────────────────────────────────────┐
   psql ───────────▶│ listener (net.Listener, TLS optional)    │
   app pool ───────▶│                                          │
                    └───────────────┬──────────────────────────┘
                                    │ one goroutine per client
                    ┌───────────────▼──────────────────────────┐
                    │ ClientSession                            │
                    │  • auth (SCRAM verifier)                 │
                    │  • fake BackendKeyData                   │
                    │  • startup params (user, db, opts)       │
                    │  • linked server conn (nil when idle)    │
                    └───────────────┬──────────────────────────┘
                                    │ acquire / release
                    ┌───────────────▼──────────────────────────┐
                    │ Pool  keyed by (user, database)          │
                    │  • buffered chan *ServerConn (free list) │
                    │  • wait queue + ctx deadline             │
                    │  • min/max size, idle reaper             │
                    └───────────────┬──────────────────────────┘
                    ┌───────────────▼──────────────────────────┐
                    │ ServerConn                               │
                    │  • real BackendKeyData                   │
                    │  • dirty flags (GUCs, prepared, temp…)   │
                    │  • prepared-stmt LRU (phase 6)           │
                    └──────────────────────────────────────────┘
                                    │
                    ┌───────────────▼──────────────────────────┐
                    │ CancelRegistry: fakeKey → ClientSession  │
                    └──────────────────────────────────────────┘
```

Proposed layout:

```
pgdoor/
├── cmd/pgdoor/main.go
├── internal/
│   ├── pgwire/          # message codec — the foundation everything sits on
│   │   ├── message.go   #   framing, read/write, direction-aware decode
│   │   ├── frontend.go  #   typed frontend messages
│   │   ├── backend.go   #   typed backend messages
│   │   └── startup.go   #   startup / SSLRequest / CancelRequest
│   ├── auth/            # SCRAM-SHA-256 both directions, md5, userlist
│   ├── proxy/           # ClientSession, relay loop, txn boundary detection
│   ├── pool/            # per-(user,db) server pools, wait queue, reaper
│   ├── cancel/          # fake key registry + cancel forwarding
│   ├── prepared/        # statement name rewriting + per-server LRU (phase 6)
│   ├── admin/           # fake "pgdoor" database: SHOW POOLS/CLIENTS/SERVERS
│   └── config/          # ini/toml, SIGHUP reload
├── testdata/            # golden captures of real wire traffic
└── docs/                # protocol notes, benchmark results
```

**Design decision to make consciously:** goroutine-per-connection with blocking reads (idiomatic
Go, simple, ~8KB/goroutine — fine at 10k clients) versus an epoll/kqueue event loop like
pgbouncer. Go's model wins on clarity; measure it before believing anyone who says otherwise.

---

## Part 5 — Phases

Each phase is independently runnable and independently demoable.

### Phase 0 — dumb TCP relay *(warm-up)*
Accept on `:6432`, dial `localhost:5432`, two `io.Copy` goroutines. `psql -p 6432` works.
**Learn:** the plumbing, graceful shutdown, half-close semantics.

### Phase 1 — the wire codec + `--trace` *(the highest-leverage phase)*
Parse every message in both directions, log a readable trace, forward bytes unchanged.
```
C→S  Query          "SELECT 1"
S→C  RowDescription 1 field: ?column? oid=23
S→C  DataRow        ["1"]
S→C  CommandComplete "SELECT 1"
S→C  ReadyForQuery  I
```
**Learn:** framing, tag ambiguity, what a real session actually looks like on the wire.
**This trace tool is how you'll debug every later phase.** Build it well.
Tests: golden files, plus a fuzzer on the framer (truncated headers, absurd lengths, 1-byte reads).

### Phase 2 — own the handshake
Terminate client auth in pgdoor (SCRAM verify against a userlist); separately authenticate to
Postgres as a client. Still strictly 1 client : 1 server connection.
**Learn:** SCRAM in both roles, `ParameterStatus` replay, why 3.5 is true.

### Phase 3 — session pooling
Per-`(user,db)` pool with min/max size, a wait queue with deadlines, an idle reaper, and health
checks. Server connections are borrowed for the life of a client connection.
**Learn:** bounded resources, backpressure, fairness, not leaking goroutines on abrupt disconnect.

### Phase 4 — transaction pooling *(the core)*
Release on `Z`/`I`. Then handle everything that makes that unsafe:
- do **not** release mid-COPY, mid-pipeline (Extended Query without `Sync`), or in `T`/`E`
- detect dirtying statements (`SET`, `LISTEN`, `PREPARE`, `CREATE TEMP`, advisory locks)
- decide your reset policy: `DISCARD ALL` on release costs a round trip; pgbouncer's guidance is
  an *empty* reset query in transaction mode with clients that behave. Measure both.
- `ParameterStatus` drift: if the server reports a changed GUC, the client must be told
**Learn:** this is the phase where you understand what a "connection" really is.

### Phase 5 — cancellation done right
Synthetic `BackendKeyData`, cancel registry, forwarding on a fresh connection.
**Test:** long `pg_sleep` in one psql, Ctrl-C it, assert a *second* client's query survives.

### Phase 6 — prepared statement emulation
Rewrite client statement names to pooler-owned names, track which server has which parsed, re-Parse
on demand, LRU-evict with `Close`. This is what makes transaction pooling usable from real drivers
(pgx, JDBC, Npgsql all use the extended protocol by default).
**Learn:** why this was pgbouncer's #1 open issue for a decade.

### Phase 7 — operability
Fake `pgdoor` admin database (`SHOW POOLS;`, `SHOW CLIENTS;`, `SHOW SERVERS;`, `PAUSE;`,
`RESUME;`), Prometheus metrics, structured logs, SIGHUP config reload.
**Learn:** re-using the very protocol you implemented to serve your own admin interface.

### Phase 8 — TLS + the benchmark that justifies the project
TLS on both sides. Then the money chart with `pgbench`:

```
pgbench -c {1,10,50,100,500,1000} -j 8 -T 60 -S
   direct  vs  pgdoor(session)  vs  pgdoor(transaction)
plot: TPS and p99 latency vs client count
```
Expect: direct wins at low concurrency (one less hop), pooled wins decisively past ~2–4× cores,
and direct falls off a cliff where pooled stays flat. **That plot is the README.**

### Stretch
Read/write splitting via `pg_stat_replication` lag checks · graceful failover · multi-tenant
`auth_query` · online restart with connection handover · sharding by key.

---

## Part 6 — Testing strategy

| Layer | Approach |
|---|---|
| codec | golden files from captured real traffic + `go-fuzz` on the framer |
| auth | RFC 5802 / RFC 7677 test vectors |
| pool | fake Postgres server (in-process `net.Pipe`) — deterministic, no DB needed |
| integration | real Postgres 17 (already running locally on :5432) driven by `psql` and `pgx` |
| behaviour | assert the *breakages*: prepared statements across a pool boundary, `LISTEN`, temp tables |
| chaos | kill server connections mid-transaction; assert the client sees a clean error, not a hang |
| perf | `pgbench` matrix, recorded in `docs/benchmarks.md` per phase |

## Part 7 — Environment (already verified on this machine)

- Go 1.24.6 (darwin/arm64)
- PostgreSQL 17.6 via Homebrew, **running on :5432**
- `psql` and `pgbench` on PATH
- Docker installed but daemon not running — start it if you want a multi-version test matrix

pgdoor listens on **:6432** (pgbouncer's conventional port).

---

## Part 8 — Decisions (locked 2026-08-25)

| Decision | Choice | Consequence |
|---|---|---|
| **Dependencies** | **Zero.** Standard library only. | Verified achievable: Go 1.24 moved `crypto/pbkdf2` and `crypto/hkdf` into the stdlib, so SCRAM-SHA-256 needs no `x/crypto`. `go.mod` should stay empty of `require` lines — treat any addition as a design failure. |
| **Working style** | **Scaffold → you implement → I review.** | Each phase ships as package layout, exported signatures, doc comments stating intent, and *failing tests*. You fill in the bodies. I review, explain, and only write code when you ask. |
| **Scope** | **Phases 0–6 plus benchmarks.** | Ends at prepared-statement emulation — the point where pgx / JDBC / Npgsql work unmodified through transaction pooling. Read/write splitting stays a documented stretch goal. |
| **Auth** | **`userlist.txt` first, `auth_query` later.** | Phase 2 verifies clients against a static `user:password` file and originates its own SCRAM exchange upstream. Phase 7 graduates to pgbouncer-style `auth_query` against `pg_shadow`. |

### What "zero deps" costs you, explicitly

You will hand-write: message framing, every typed message, SCRAM-SHA-256 as **both** verifier and
client, `SASLprep`-lite normalisation, the cancel-key registry, the pool, and the LRU. You get
`crypto/tls`, `crypto/pbkdf2`, `crypto/hmac`, `crypto/sha256`, `crypto/rand`, `encoding/binary`,
`net`, and nothing else. That is the point.

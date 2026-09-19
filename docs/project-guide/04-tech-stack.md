# Tech Stack

## Languages and runtimes

| Language or runtime | Version | Evidence |
| --- | --- | --- |
| Go | Minimum 1.24; exact toolchain patch not pinned | [go.mod:3](../../go.mod#L3) |
| PostgreSQL wire protocol | v3 constant `196608`; codec implementation pending | [message.go:29](../../internal/pgwire/message.go#L29) |
| PostgreSQL server | External; 17.6 documented as a historical local environment, not a dependency pin or tested compatibility matrix | [docs/benchmarks.md:17](../benchmarks.md#L17) |

The project has no `require` block in [go.mod](../../go.mod), consistent with the explicit standard-library-only policy in [README.md:62](../../README.md#L62). The plan's references to SCRAM cryptographic packages describe future work; the current code does not import or implement them ([PLAN.md:385](../../PLAN.md#L385)).

## Frameworks and major libraries

No external framework is used. All implemented libraries below are Go standard-library packages, versioned with the chosen toolchain rather than separately pinned.

| Library | Version | Used for | Used in |
| --- | --- | --- | --- |
| `net` | Go standard library | TCP listening, accepting, upstream dialing, no-delay, connection interfaces | [main.go:14](../../cmd/pgdoor/main.go#L14), [relay.go:13](../../internal/proxy/relay.go#L13) |
| `io` | Go standard library | Raw bidirectional copying and reader/writer contracts | [relay.go:53](../../internal/proxy/relay.go#L53), [message.go:21](../../internal/pgwire/message.go#L21) |
| `sync` | Go standard library | Process/session WaitGroups and error mutex | [main.go:56](../../cmd/pgdoor/main.go#L56), [relay.go:35](../../internal/proxy/relay.go#L35) |
| `context`, `os/signal`, `syscall` | Go standard library | SIGINT/SIGTERM listener shutdown | [main.go:46](../../cmd/pgdoor/main.go#L46) |
| `flag`, `time` | Go standard library | Startup flags and dial duration | [main.go:30](../../cmd/pgdoor/main.go#L30) |
| `log`, `os`, `errors` | Go standard library | Stderr logs and error classification | [main.go:38](../../cmd/pgdoor/main.go#L38), [relay.go:40](../../internal/proxy/relay.go#L40) |
| `bufio` | Go standard library | Actual 32 KiB constructors supporting the future codec | [message.go:120](../../internal/pgwire/message.go#L120), [message.go:151](../../internal/pgwire/message.go#L151) |
| `testing`, `bytes`, `strings` | Go standard library | Fixture assertions, TODO recovery, fuzz seeds | [pgwire_test.go:3](../../internal/pgwire/pgwire_test.go#L3) |

## Data and infrastructure

| Facility | Role | Configured at |
| --- | --- | --- |
| TCP listener | Accept client connections, default `:6432` | [main.go:31](../../cmd/pgdoor/main.go#L31) |
| External PostgreSQL TCP endpoint | One upstream session per client, default `localhost:5432` | [main.go:32](../../cmd/pgdoor/main.go#L32), [main.go:92](../../cmd/pgdoor/main.go#L92) |
| stderr | Lifecycle/error logs; intended message summaries in trace mode | [main.go:38](../../cmd/pgdoor/main.go#L38), [relay.go:142](../../internal/proxy/relay.go#L142) |

There is no local datastore, pool service, metrics endpoint, auth file reader, or configuration-reload subsystem in the [tracked source map](03-structure.md). The ignored `userlist.txt` name anticipates later auth work; ignoring a filename does not mean the executable loads it ([.gitignore:5](../../.gitignore#L5), [PLAN.md:383](../../PLAN.md#L383)).

## Tooling

| Tool | Role | Configured/documented at |
| --- | --- | --- |
| `make` / `go build` | Build `bin/pgdoor` and run with default endpoints | [Makefile:3](../../Makefile#L3) |
| `go vet` | Static analysis; a successful result does not imply TODO methods work | [Makefile:15](../../Makefile#L15) |
| `go test` | Current failing codec specifications and fuzz seeds | [Makefile:12](../../Makefile#L12), [pgwire_test.go:16](../../internal/pgwire/pgwire_test.go#L16) |
| Go built-in fuzzing | `FuzzReadMessage`, requested 60-second run once seed checks pass | [Makefile:18](../../Makefile#L18), [pgwire_test.go:359](../../internal/pgwire/pgwire_test.go#L359) |
| `psql` | Manual PostgreSQL demo through the raw relay | [README.md:33](../../README.md#L33) |
| `pgbench` | Planned throughput/latency comparison; no recorded results | [docs/benchmarks.md:3](../benchmarks.md#L3), [docs/benchmarks.md:21](../benchmarks.md#L21) |

## Notes

The source inventory has no CI workflow, container file, deployment manifest, or tool-version manager. Tooling instructions are local development commands, not evidence that CI or PostgreSQL provisioning exists ([structure](03-structure.md)). Likewise, the historical machine description in [PLAN.md:365](../../PLAN.md#L365) does not establish that a server is running on another checkout's host. See the [guide verification record](README.md#verification-of-this-guide) for checks actually performed for this revision.

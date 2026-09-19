# Decisions

This repository records both implemented choices and future intentions. The distinction matters because the tests and panic stubs are deliberately part of the learning workflow, not unfinished work accidentally hidden by a successful build.

## Keep the scaffold as the learning interface

- **What:** exported signatures, protocol comments, constants, and failing tests precede method bodies. Most codec operations explicitly panic with a phase identifier.
- **Evidence:** [PLAN.md:381](../../PLAN.md#L381), [message.go:137](../../internal/pgwire/message.go#L137), [startup.go:94](../../internal/pgwire/startup.go#L94), [pgwire_test.go:16](../../internal/pgwire/pgwire_test.go#L16).
- **Why:** the plan states “Scaffold → you implement → I review,” making implementation the learner's work.
- **Tradeoff:** the binary compiles while trace fails at runtime; tests intentionally fail until the next learning slice is implemented. Documentation must identify that boundary clearly.
- **Confidence:** explicitly recorded project decision, verified in code.

## Start with opaque copying and one upstream per client

- **What:** one handler dials one server, then runs two `io.Copy` directions without decoding protocol state.
- **Evidence:** [main.go:92](../../cmd/pgdoor/main.go#L92), [relay.go:34](../../internal/proxy/relay.go#L34), [PLAN.md:285](../../PLAN.md#L285).
- **Why:** source comments describe proving transport, half-closes, and shutdown before introducing protocol knowledge.
- **Tradeoff:** preserves the bytes of existing PostgreSQL interactions but provides none of the planned connection reduction. One client still consumes one upstream connection and two proxy sockets.
- **Confidence:** rationale stated in comments and plan; resource cost inferred from actual connections/goroutines.

## Use goroutines and half-closes

- **What:** run a copy goroutine in each direction, half-close each destination when its source ends, and wait for both before returning.
- **Evidence:** [relay.go:51](../../internal/proxy/relay.go#L51), [relay.go:63](../../internal/proxy/relay.go#L63), [PLAN.md:275](../../PLAN.md#L275).
- **Why:** half-closing allows reverse-direction bytes to drain; the plan prefers Go's blocking/goroutine model for clarity over an explicit event loop.
- **Tradeoff:** completion depends on both peers, and the current implementation installs no read/write deadline or shared cancellation on the first failure. Non-half-close connections receive a full close instead.
- **Confidence:** stated comments for drainage; wait behavior verified from code.

## Keep raw bodies and direction in the codec API

- **What:** `RawMessage` holds tag/body rather than an eagerly decoded object tree; tag descriptions take an explicit direction.
- **Evidence:** [message.go:67](../../internal/pgwire/message.go#L67), [tags.go:3](../../internal/pgwire/tags.go#L3), [pgwire_test.go:207](../../internal/pgwire/pgwire_test.go#L207).
- **Why:** most traffic will be forwarded unchanged, and identical tag bytes mean different things in the two directions.
- **Tradeoff:** callers must supply direction correctly and only selectively interpret bodies. Body ownership/reuse remains undecided, and all relevant codec operations are still stubs.
- **Confidence:** intended rationale stated in comments; not yet an implemented decoding guarantee.

## Separate the first packet and flush forwarded messages

- **What:** `ReadFirstPacket` handles untagged startup/special requests separately; normal trace forwarding plans to write and flush every message before reading the next one.
- **Evidence:** [startup.go:3](../../internal/pgwire/startup.go#L3), [relay.go:78](../../internal/proxy/relay.go#L78), [relay.go:133](../../internal/proxy/relay.go#L133).
- **Why:** startup does not use tagged framing, SSL negotiation has a one-byte response, and buffered request/response traffic can deadlock if not flushed before the next blocking read.
- **Tradeoff:** special handling and a buffered-reader handoff problem at encryption fallback; eager flushes prioritize progress over write batching. This path is currently unreachable beyond the first parser call.
- **Confidence:** intended protocol/flush rationale documented; buffer handoff remains an [open question](README.md#open-questions).

## Depend only on the Go standard library

- **What:** no module requirements beyond Go itself; future protocol/auth/pool code is intended to be written in-repository.
- **Evidence:** [go.mod](../../go.mod), [README.md:62](../../README.md#L62), [PLAN.md:380](../../PLAN.md#L380).
- **Why:** the documented objective is understanding and implementing these mechanisms from scratch; the plan identifies standard-library cryptographic building blocks for later phases.
- **Tradeoff:** more protocol/authentication implementation and verification work. Planned availability of crypto primitives does not imply any auth mechanism is currently implemented.
- **Confidence:** explicit project policy.

## Gotchas

- **The introduction describes the destination.** The opening [README.md:8](../../README.md#L8) talks about transaction multiplexing, while the [status table](../../README.md#L20) and [fresh upstream dial](../../cmd/pgdoor/main.go#L92) show only phase 0 works today. No `Pool` or `ClientSession` implementation exists.
- **Trace can terminate the process even before receiving client bytes.** After upstream dial, `ReadFirstPacket` panics immediately, and connection goroutines do not recover ([startup.go:94](../../internal/pgwire/startup.go#L94), [main.go:67](../../cmd/pgdoor/main.go#L67)). `make trace` enables that path.
- **“Until either side closes” is incomplete.** The relay comment says this, but `wg.Wait` requires both directions to finish. Its error filter also ignores `net.ErrClosed`, in addition to EOF ([relay.go:30](../../internal/proxy/relay.go#L30), [relay.go:40](../../internal/proxy/relay.go#L40), [relay.go:66](../../internal/proxy/relay.go#L66)).
- **EOF is not a generated protocol message.** `CloseWrite` signals TCP EOF; the relay does not encode PostgreSQL `Terminate`, despite the analogy in its half-close comment ([relay.go:23](../../internal/proxy/relay.go#L23), [relay.go:55](../../internal/proxy/relay.go#L55)).
- **Encryption request is not acceptance.** The trace scaffold falls back without inspecting the response byte, so its “encrypted from here” log would also appear if the server refused encryption. Raw mode simply forwards whatever peers negotiate ([relay.go:110](../../internal/proxy/relay.go#L110)). The constant comment saying pgdoor replies `N` to GSS is future intent, not code in the current relay ([message.go:40](../../internal/pgwire/message.go#L40)).
- **Buffered startup must not strand bytes.** `NewReader` uses `bufio`, but the raw fallback bypasses that wrapper. The no-overread test specifically expects trailing bytes to remain in the underlying reader; its prose about exposing a buffered remainder is not matched by a public handoff API ([message.go:120](../../internal/pgwire/message.go#L120), [relay.go:114](../../internal/proxy/relay.go#L114), [pgwire_test.go:332](../../internal/pgwire/pgwire_test.go#L332)).
- **Declared limits do not protect raw mode.** `MaxMessageSize` is a pending parser requirement. `Relay` has no notion of message size, and the current parser panics rather than checking lengths ([message.go:44](../../internal/pgwire/message.go#L44), [message.go:137](../../internal/pgwire/message.go#L137)).
- **Signal handling has no forced-drain deadline.** Closing the listener leaves existing connections alive and waits for their disconnections; the dial timeout is not a session timeout ([main.go:46](../../cmd/pgdoor/main.go#L46), [main.go:73](../../cmd/pgdoor/main.go#L73)).
- **Benchmark expectations are not measurements.** [docs/benchmarks.md:11](../benchmarks.md#L11) states a hypothesis and [its results section](../benchmarks.md#L21) explicitly records no results.

## Conventions

Keep transport code under `internal/proxy` and wire details under `internal/pgwire`; identify learning-stage work with `TODO(phase-N)` rather than describing its contract as already implemented ([relay.go:20](../../internal/proxy/relay.go#L20), [startup.go:93](../../internal/pgwire/startup.go#L93)). The existing tests use hand-assembled bytes and named table cases; extend those contracts when implementing the codec ([pgwire_test.go:24](../../internal/pgwire/pgwire_test.go#L24), [pgwire_test.go:124](../../internal/pgwire/pgwire_test.go#L124)). Preserve direction in tag interpretation and separate actual verification results from roadmap assertions throughout documentation.

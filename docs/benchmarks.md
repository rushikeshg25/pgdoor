# Benchmarks

Recorded per phase. The headline chart (phase 8) is TPS and p99 latency versus client
count, for three configurations against the same Postgres:

```
pgbench -c {1,10,50,100,500,1000} -j 8 -T 60 -S
  direct  |  pgdoor (session pooling)  |  pgdoor (transaction pooling)
```

Expectation: direct wins at low concurrency (one less network hop), pooled wins
decisively past roughly `2–4 × cores` active connections, and direct falls off a cliff
where pooled stays flat.

## Environment

- Apple Silicon (darwin/arm64), Go 1.24.6
- PostgreSQL 17.6 (Homebrew), local, `:5432`
- pgdoor on `:6432`

## Results

_Nothing recorded yet — phase 0 is a pass-through relay, so any measurement here would
only be measuring the extra hop._

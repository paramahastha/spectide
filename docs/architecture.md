# Architecture

The Go process is the single workflow authority. It stores durable run state and ordered redacted events in SQLite, serves loopback HTTP/SSE, and embeds the dashboard. Provider adapters produce only explicit argument-vector invocations; they never read or copy credentials. The simulated adapter is the required offline path and is labeled `SIMULATED`.

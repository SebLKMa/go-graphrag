# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Goal

A simple GraphRAG (Graph Retrieval-Augmented Generation) system backed by a graph database (Neo4j or Memgraph), in Go. The goal is to "talk" to the graph: ask a natural-language question, have an LLM translate it into Cypher, run it, and show both the query and its results. Full briefs: `NEO4J.md`, `MEMGRAPH.md`.

## Commands

```sh
go build ./...                    # build everything
go vet ./...                      # vet
go run ./cmd/ingest               # load datapipe/*.csv into the graph database
go run ./cmd/chat                 # interactive NL -> Cypher REPL against the graph
go run ./cmd/ingest -db memgraph  # same, against Memgraph instead of Neo4j
```

There is no test suite yet.

`cmd/ingest` and `cmd/chat` both take a `-db neo4j|memgraph` flag (default: `$GRAPH_DB`, else `neo4j`) selecting the backend, which sets the connection defaults:
- `neo4j`: `bolt://localhost:7687`, user `neo4j`, password `graph4fun` (matching the Docker setup below).
- `memgraph`: `bolt://localhost:7687`, no auth (empty user/password -> `neo4j.NoAuth()`).
- Overrides: `GRAPH_URI`/`GRAPH_USER`/`GRAPH_PASSWORD` env vars (set-but-empty counts, so empty credentials are expressible); legacy `NEO4J_URI`/`NEO4J_USER`/`NEO4J_PASSWORD` honored as fallback; `cmd/ingest` also has `-uri`/`-user`/`-password` flags.
- `cmd/chat` additionally requires `ANTHROPIC_API_KEY`, and reads `ANTHROPIC_MODEL` (default `claude-sonnet-5`).

Both backends use Bolt port 7687, so only one of them can be running at a time (they'd otherwise collide on the port); the ingest/chat code is identical across the two — same driver, same Cypher.

## Neo4j

`neo4j/start.sh` starts (or reuses) a `my-neo4j` Docker container per `NEO4J.md`: ports 7474/7687, volumes under `$HOME/neo4j/{data,logs,import,plugins}`, auth `neo4j/graph4fun`. Browser at http://localhost:7474.

## Memgraph

`memgraph/start.sh` starts (or reuses) two containers per `MEMGRAPH.md`, on the `memgraph-net` Docker network (created if missing): `memgraph` (memgraph/memgraph:2.11.0 — the version that works on Windows VMware Ubuntu; ports 7687/7444, data under `$HOME/memgraph-data/lib`, no auth) and `memgraph-lab` (browser UI at http://localhost:3000). `memgraph/stop.sh` stops and removes both.

`Makefile-neo4j` and `Makefile-mg` build `cmd/ingest` and `cmd/chat` into `bin/neo4j` and `bin/mg` respectively (`make -f Makefile-mg`).

## Architecture

- `internal/hierarchy` — parses the slash-delimited equipment paths used in the CSVs (e.g. `STN/TESTSTATION-3/SWB/TESTBOARD-1/PNL/TESTPANEL-5`) into a chain of typed nodes (`Station`, `Switchboard`, `Panel`, `Sensor`), stopping at the first unrecognized type code. This is reused for both `equipment_id` (equipment hierarchy) and inspection `source_path` (which extends the same path down through a `Sensor` and trailing date/timestamp segments that aren't parsed).
- `internal/graphdb` — thin wrapper around the Neo4j Bolt driver (`neo4j.ExecuteQuery`/`EagerResultTransformer`), which serves both backends; both ingestion and chat go through `DB.Run`. Backend selection/defaults live in `internal/config` (`GraphBackend`).
- `internal/ingest` — reads the CSVs (`csv.go`, header-keyed rows), builds per-row property maps (`props.go`, skipping empty cells rather than storing zero values), and writes the graph (`ingest.go`). `mergeChain` MERGEs a hierarchy chain plus `:HAS` edges between consecutive levels; it's idempotent so equipment and inspection ingestion can both call it for overlapping path prefixes.
- `internal/llm` — direct HTTP client for the Anthropic Messages API (no SDK dependency); `CypherForQuestion` sends a schema description + system prompt instructing the model to return exactly one read-only Cypher query, targeting the selected backend's dialect (`Client.Dialect`).
- `cmd/ingest` — loads `datapipe/equipments.csv` then `datapipe/equipment_health_inspections.csv`.
- `cmd/chat` — REPL: question -> `llm.CypherForQuestion` -> print Cypher -> `graphdb.DB.Run` -> print results (with `neo4j.Node`/`neo4j.Relationship` given readable `(:Label {props})` formatting).

## Graph schema

```
(:Station {id, name})-[:HAS]->(:Switchboard {id, name})-[:HAS]->(:Panel:Equipment {...})-[:HAS]->(:Sensor {id, name})
(:Inspection {...})-[:INSPECTS]->(:Panel:Equipment)
(:Inspection {...})-[:VIA]->(:Sensor)
```

`id` on every node is the cumulative slash-delimited path up to that segment (e.g. a Panel's id is its full `equipment_id`), so the same physical station/switchboard/panel/sensor referenced from different CSV rows merges onto one node. `Panel` nodes additionally get an `:Equipment` label and the full set of `equipments.csv` columns as properties (see `equipmentProps` in `internal/ingest/props.go`); `Inspection` nodes get the `equipment_health_inspections.csv` columns (`inspectionProps`).

## Data model (`datapipe/`)

- `equipments.csv` — one row per piece of equipment (Panel). `equipment_id` is the hierarchical path; other columns become Panel properties.
- `equipment_health_inspections.csv` — one row per inspection. `equipment_id` links back to the inspected Panel; `source_path` extends that same path down to the Sensor that took the reading.

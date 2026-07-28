# Creating a simple GraphRag using neo4j

The programming language of choice is Go.

## Goal

The goal is to be able to "talk" to the GraphRAG.
Show the queries and responses of the entities in the graph.

## Start memgraph as docker container

Skip this step if `memgraph` is already running in `docker ps -a`
(the script is idempotent and reuses running containers anyway).

```sh
./memgraph/start.sh
```

Memgraph listens on `bolt://localhost:7687` with no auth;
Memgraph Lab (browser UI) is at http://localhost:3000.
Stop and remove both containers with `./memgraph/stop.sh`.

## Ingest data

Review the CSV files in `./datapipe` directory.
The equipments.csv contains the equipments.
The equipment_health_inspections.csv contains the inspections of the equipments.

Construct a simple GraphRAG using the ingested data in memgraph:

```sh
go run ./cmd/ingest -db memgraph
```

## Talk to the graph

```sh
export ANTHROPIC_API_KEY=...   # or source .env
go run ./cmd/chat -db memgraph
```

`-db memgraph` can also be set persistently with `export GRAPH_DB=memgraph`.



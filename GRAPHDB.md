# Creating a simple GraphRag using GraphDB

The programming language of choice is Go.

## Goal

The goal is to be able to "talk" to the GraphRAG.
Show the queries and responses of the entities in the graph.

## Start graphdb as docker container

Skip this step if `graphdb` is already running in `docker ps -a`
(the script is idempotent and reuses running containers anyway).

```sh
./graphdb/start.sh
```

## Ingest data

Review the CSV files in `./datapipe` directory.
The equipments.csv contains the equipments.
The equipment_health_inspections.csv contains the inspections of the equipments.

Construct a simple GraphRAG using the ingested data in graphdb:

```sh
go run ./cmd/ingest -db graphdb
```

## Talk to the graph

```sh
export ANTHROPIC_API_KEY=...   # or source .env
go run ./cmd/chat -db graphdb
```

`-db graphdb` can also be set persistently with `export GRAPH_DB=graphdb`.



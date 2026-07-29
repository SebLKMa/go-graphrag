# Creating a simple GraphRag using GraphDB

The programming language of choice is Go.

## Goal

The goal is to be able to "talk" to the GraphRAG.
Show the queries and responses of the entities in the graph.

Unlike the Neo4j and Memgraph backends (Bolt + Cypher), Ontotext GraphDB is
an RDF triple store: ingestion writes triples over its HTTP REST API, and
questions are translated to SPARQL instead of Cypher.

## Start graphdb as docker container

Skip this step if `graphdb` is already running in `docker ps -a`
(the script is idempotent and reuses running containers anyway).

```sh
./graphdb/start.sh
```

The image is pinned to `ontotext/graphdb:10.8.1` — GraphDB 11+ refuses
writes without a license file, while the 10.x free mode runs unlicensed.
The workbench UI is at http://localhost:7200. `./graphdb/stop.sh` stops and
removes the container.

## Ingest data

Review the CSV files in `./datapipe` directory.
The equipments.csv contains the equipments.
The equipment_health_inspections.csv contains the inspections of the equipments.

Construct a simple GraphRAG using the ingested data in graphdb:

```sh
go run ./cmd/ingest -db graphdb
```

This creates the `graphrag` repository if needed (override with
`-repository` or `GRAPH_REPOSITORY`) and loads both CSVs as RDF triples:
each station/switchboard/panel/sensor becomes a resource under
`http://graphrag.example/entity/`, typed `eq:Station`/`eq:Switchboard`/
`eq:Panel` (+`eq:Equipment`)/`eq:Sensor`/`eq:Inspection` with `eq:has`,
`eq:inspects` and `eq:via` links and the CSV columns as `eq:*` properties.
Re-running is idempotent (duplicate triples are no-ops).

## Talk to the graph

```sh
export ANTHROPIC_API_KEY=...   # or source .env
go run ./cmd/chat -db graphdb
```

Each question is translated by the LLM into one read-only SPARQL query,
which is printed together with its results:

```
> which stations have inspected panels, and what was the qmax value of each inspection?
SPARQL: PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX eq: <http://graphrag.example/schema#>
SELECT ?stationName ?panelName ?qmax_value WHERE { ... }
stationName=TESTSTATION-3  panelName=TESTPANEL-5  qmax_value=5.775419999999999E+00
stationName=TESTSTATION-4  panelName=TESTPANEL-1  qmax_value=1.1716184219591838E+02
```

`-db graphdb` can also be set persistently with `export GRAPH_DB=graphdb`.

## Build binaries

```sh
make -f Makefile-graphdb   # builds bin/graphdb/{ingest,chat}
```



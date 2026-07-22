# Creating a simple GraphRag using neo4j

The programming language of choice is Go.

## Goal

The goal is to be able to "talk" to the GraphRAG.
Show the queries and responses of the entities in the graph.

## Start neo4j as docker container

Skip this step if `my-neo4j` is already running in `docker ps -a`.

```sh
docker run \
    --name my-neo4j \
    -p 7474:7474 -p 7687:7687 \
    -d \
    -v $HOME/neo4j/data:/data \
    -v $HOME/neo4j/logs:/logs \
    -v $HOME/neo4j/import:/import \
    -v $HOME/neo4j/plugins:/plugins \
    -e NEO4J_AUTH=neo4j/graph4fun \
    neo4j:latest
```

# Open browser and navigate to the official local interface at http://localhost:7474.
# You should be able to log in using the username neo4j and the password you defined above.

## Ingest data

Review the CSV files in `./datapipe` directory.
The equipments.csv contains the equipments.
The equipment_health_inspections.csv contains the inspections of the equipments.

Construct a simple GraphRAG using the ingested data.



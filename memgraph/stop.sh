#!/usr/bin/env bash
# Stops and removes the Memgraph + Memgraph Lab containers started by start-mg.sh.
docker stop memgraph-lab
docker stop memgraph
docker rm memgraph-lab
docker rm memgraph

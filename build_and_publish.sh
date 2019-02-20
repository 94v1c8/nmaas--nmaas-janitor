#!/bin/bash

TAG=LATEST
WHAT=janitor
sudo docker build --rm -t artifactory.geant.net/nmaas-docker-local/nmaas-$WHAT:$TAG .
sudo docker push artifactory.geant.net/nmaas-docker-local/nmaas-$WHAT:$TAG

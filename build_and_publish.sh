#!/bin/bash

TAG=1.3.0
WHAT=janitor
sudo docker build --rm -t artifactory.software.geant.org/nmaas-docker-local/nmaas-$WHAT:$TAG .
sudo docker push artifactory.software.geant.org/nmaas-docker-local/nmaas-$WHAT:$TAG

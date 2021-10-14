#!/bin/bash

TAG=1.4.4
WHAT=janitor
sudo docker build --rm -t artifactory.software.geant.org/nmaas-docker-local/nmaas-$WHAT:$TAG .
sudo docker push artifactory.software.geant.org/nmaas-docker-local/nmaas-$WHAT:$TAG

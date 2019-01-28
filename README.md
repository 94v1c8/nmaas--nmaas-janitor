# NMaaS Janitor - microservice that controls deployments
## Used technologies
* Go
* Protobuf
* gRPC
* kubernetes/client-go
* go-gitlab

## Features
* Creating deployment ConfigMap when configuration is loaded on gitlab
* Updating deployment ConfigMap on demand

## Deploying
The provided docker image is a two-stage build image. 
You don't have to compile protoc yourself, nor configure local golang environment. Just run `docker build`, and image will do all the work for you

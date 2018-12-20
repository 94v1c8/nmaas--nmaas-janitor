# NMaaS Janitor - microservice that controls deployments
## Used technologies
* Go
* Protobuf
* gRPC
* kubernetes/client-go
* go-gitlab
## Features
* Updating deployment ConfigMap on-demand

## Build requirements
go >= 1.11\
protoc  == 3.6.0 (BEWARE! do not use 3.6.1 or any other "latest" versions)

## Building
From working directory, perform:

    ./third-party/protoc-gen.sh
    cd pkg/cmd/server
    go build .
    
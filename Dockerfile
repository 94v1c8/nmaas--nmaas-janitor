FROM golang:1.17.2-buster as builder
WORKDIR /
RUN apt-get update && apt-get install unzip
RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v3.13.0/protoc-3.13.0-linux-x86_64.zip
RUN unzip protoc-3.13.0-linux-x86_64.zip -d /
WORKDIR /build
COPY api/ api
COPY pkg/ pkg
COPY third_party/ third_party
COPY go.mod/ .

RUN go get google.golang.org/grpc
RUN go get github.com/johnaoss/htpasswd/apr1
RUN go get github.com/xanzy/go-gitlab
RUN go get k8s.io/api/core/v1
RUN go get k8s.io/apimachinery/pkg/apis/meta/v1
RUN go get k8s.io/apimachinery/pkg/types
RUN go get k8s.io/client-go/kubernetes
RUN go get k8s.io/client-go/rest
RUN go get github.com/evanphx/json-patch
RUN go get github.com/golang/protobuf/protoc-gen-go
RUN go get k8s.io/kube-openapi/pkg/util/proto

RUN mkdir -p /build/pkg/api/v1
RUN protoc --proto_path=/build/api/proto/v1 --proto_path=/build/third_party --go_out=plugins=grpc:/build/pkg/api/v1 config-service.proto
RUN CGO_ENABLED=0 GOOS=linux go test ./...
WORKDIR /build/pkg/cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:latest
MAINTAINER nmaas@lists.geant.org
COPY --from=builder /build/pkg/cmd/server/server /go/bin/nmaas-janitor
ENTRYPOINT /go/bin/nmaas-janitor -port $SERVER_PORT -token $GITLAB_TOKEN -url $GITLAB_URL

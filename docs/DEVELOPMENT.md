# NMaaS Janitor

### Technologies

* [Go](https://go.dev/)
* [Protobuf](https://protobuf.dev/)
* [gRPC](https://grpc.io/)
* [kubernetes/client-go](https://github.com/kubernetes/client-go)
* [go-gitlab](https://pkg.go.dev/github.com/xanzy/go-gitlab)

### Deploying

The provided Dockerfile comprises a two-stage Docker image build.
You don't have to compile protoc yourself, nor configure local golang environment. Just run `docker build`, and image will do all the work for you.
# NMaaS Janitor (back-end)

#### Microservice that interacts with GitLab and Kubernetes API to perform various low level operations on behalf of the NMaaS Platform

### Technologies

* Go
* Protobuf
* gRPC
* kubernetes/client-go
* go-gitlab

### Features

* Creating deployment ConfigMap(s) when configuration is pushed to GitLab repository
* Updating deployment ConfigMap(s) on demand
* Verifying deployment or statefulset status on demand
* Setting basic auth parameters on Ingress resources on demand
* Retrieving loadbalancer IP address assigned to given deployment or statefulset

### Deploying

The provided docker image is a two-stage build image. 
You don't have to compile protoc yourself, nor configure local golang environment. Just run `docker build`, and image will do all the work for you.

package v1

import (
	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
	"context"
	extension "k8s.io/api/extensions/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"testing"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestCheckAPI(t *testing.T) {
	api := "wrong"
	current := "correct"
	err := checkAPI(api, current)
	if err == nil {
		t.Fail()
	}

	api = "correct"
	err = checkAPI(api, current)
	if err != nil {
		t.Fail()
	}
}

var inst = v1.Instance{Namespace: "test-namespace", Uid: "test-uid", Domain: "test-domain"}
var fake_ns_inst = v1.Instance{Namespace: "fake-namespace", Uid: "test-uid", Domain: "test-domain"}

var req = v1.InstanceRequest{Api: apiVersion, Deployment: &inst}
var illegal_req = v1.InstanceRequest{Api: "illegal", Deployment: &inst}

func TestReadinessServiceServer_CheckIfReady(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewReadinessServiceServer(client)

	//Fail on API version check
	res, err := server.CheckIfReady(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.CheckIfReady(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(&ns)

	//Fail on deployment
	res, err = server.CheckIfReady(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock deployment
	depl := extension.Deployment{}
	depl.Name = "test-uid"
	q := int32(5)
	depl.Spec.Replicas = &q
	depl.Status.Replicas = q
	_, _ = client.ExtensionsV1beta1().Deployments("test-namespace").Create(&depl)


}

func TestCertManagerServiceServer_DeleteIfExists(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewCertManagerServiceServer(client)

	//Fail on API version check
	res, err := server.DeleteIfExists(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.DeleteIfExists(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(&ns)

	//Pass if already nonexistent
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}
}

func TestBasicAuthServiceServer_DeleteIfExists(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewBasicAuthServiceServer(client)

	res, err := server.DeleteIfExists(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}
}
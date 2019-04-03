package v1

import (
	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
	"context"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	_, _ = client.CoreV1().Namespaces().Create(&corev1.Namespace{
		ObjectMeta: v12.ObjectMeta {
			Name: "test-namespace",
		},
	})

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.CheckIfReady(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//Fail on deployment

}

func TestCertManagerServiceServer_DeleteIfExists(t *testing.T) {
	server := NewCertManagerServiceServer(testclient.NewSimpleClientset())

	res, err := server.DeleteIfExists(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}
}
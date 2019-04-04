package v1

import (
	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
	"context"
	"github.com/xanzy/go-gitlab"
	extension "k8s.io/api/extensions/v1beta1"
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

	//create mock deployment that is fully deployed
	depl := extension.Deployment{}
	depl.Name = "test-uid"
	q := int32(5)
	depl.Spec.Replicas = &q
	depl.Status.ReadyReplicas = q
	_, _ = client.ExtensionsV1beta1().Deployments("test-namespace").Create(&depl)

	res, err = server.CheckIfReady(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//modify mock deployment to be partially deployed
	p := int32(3)
	depl.Status.ReadyReplicas = p
	_, _ = client.ExtensionsV1beta1().Deployments("test-namespace").Update(&depl)

	res, err = server.CheckIfReady(context.Background(), &req)
	if err != nil || res.Status != v1.Status_PENDING {
		t.Fail()
	}
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

	//Create mock secret
	sec := corev1.Secret{}
	sec.Name = "test-uid-tls"
	_, _ = client.CoreV1().Secrets("test-namespace").Create(&sec)

	//Pass
	res, err = server.DeleteIfExists(context.Background(), &req)
}

func TestBasicAuthServiceServer_DeleteIfExists(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewBasicAuthServiceServer(client)

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

	//Create mock secret
	sec := corev1.Secret{}
	sec.Name = getAuthSecretName("test-uid")
	_, _ = client.CoreV1().Secrets("test-namespace").Create(&sec)

	//Pass
	res, err = server.DeleteIfExists(context.Background(), &req)
}

func TestBasicAuthServiceServer_CreateOrReplace(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewBasicAuthServiceServer(client)

	creds := v1.Credentials{User: "test-user", Password: "test-password"}

	//Fail on api test
	illreq := v1.InstanceCredentialsRequest{Api: "dummy", Instance: &fake_ns_inst, Credentials: &creds}
	res, err := server.CreateOrReplace(context.Background(), &illreq)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceCredentialsRequest{Api:apiVersion, Instance:&fake_ns_inst, Credentials: &creds}
	res, err = server.CreateOrReplace(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(&ns)

	//Should create new secret
	req := v1.InstanceCredentialsRequest{Api:apiVersion, Instance:&inst, Credentials: &creds}
	res, err = server.CreateOrReplace(context.Background(), &req)
	if res.Status != v1.Status_OK || err != nil {
		t.Fail()
	}

	sec, err := client.CoreV1().Secrets("test-namespace").Get(getAuthSecretName("test-uid"), v12.GetOptions{})
	if err != nil || sec == nil {
		t.Fail()
	}

	//Should update secret when already exists
	res, err = server.CreateOrReplace(context.Background(), &req)
	if res.Status != v1.Status_OK || err != nil {
		t.Fail()
	}
}

func TestConfigServiceServer_DeleteIfExists(t *testing.T) {
	client := testclient.NewSimpleClientset()
	gitclient := gitlab.Client{}
	server := NewConfigServiceServer(client, &gitclient)

	//Should fail on api check
	res, err := server.DeleteIfExists(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Should fail on namespace check
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(&ns)

	//Should return ok on configmap check if missing
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//create mock configmap
	cm := corev1.ConfigMap{}
	cm.Name = "test-uid"
	_, _ = client.CoreV1().ConfigMaps("test-namespace").Create(&cm)

	//should pass on deleting existing configmap
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}
}
package v1

import (
	v1 "bitbucket.software.geant.org/projects/NMAAS/repos/nmaas-janitor/pkg/api/v1"
	"context"
	"github.com/xanzy/go-gitlab"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	testclient "k8s.io/client-go/kubernetes/fake"
	"fmt"
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
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Fail on deployment
	res, err = server.CheckIfReady(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock deployment that is fully deployed
	depl := appsv1.Deployment{}
	depl.Name = "test-uid"
	q := int32(5)
	depl.Spec.Replicas = &q
	depl.Status.ReadyReplicas = q
	_, _ = client.AppsV1().Deployments("test-namespace").Create(context.Background(), &depl, metav1.CreateOptions{})

	res, err = server.CheckIfReady(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//modify mock deployment to be partially deployed
	p := int32(3)
	depl.Status.ReadyReplicas = p
	_, _ = client.AppsV1().Deployments("test-namespace").Update(context.Background(), &depl, metav1.UpdateOptions{})

	res, err = server.CheckIfReady(context.Background(), &req)
	if err != nil || res.Status != v1.Status_PENDING {
		t.Fail()
	}
}

func TestReadinessServiceServer_CheckIfReadyWithStatefulSet(t *testing.T) {
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
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Fail on missing deployment
	res, err = server.CheckIfReady(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock statefulset that is fully deployed
	sts := appsv1.StatefulSet{}
	sts.Name = "test-uid"
	q := int32(5)
	sts.Spec.Replicas = &q
	sts.Status.ReadyReplicas = q
	_, _ = client.AppsV1().StatefulSets("test-namespace").Create(context.Background(), &sts, metav1.CreateOptions{})

	res, err = server.CheckIfReady(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}
}

func TestInformationServiceServer_RetrieveServiceIp(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewInformationServiceServer(client)

	//Fail on API version check
	res, err := server.RetrieveServiceIp(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.RetrieveServiceIp(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Fail on loading services
	res, err = server.RetrieveServiceIp(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED || res.Message != "Service not found!" {
		t.Fail()
	}

	//create mock service without ingress
	s1 := corev1.Service{}
	s1.Name = "test-uid"
	_, _ = client.CoreV1().Services("test-namespace").Create(context.Background(), &s1, metav1.CreateOptions{})

	//Fail on missing service ingress
	res, err = server.RetrieveServiceIp(context.Background(), &req)
	if err != nil || res.Status != v1.Status_FAILED || res.Message != "Service ingress not found!"{
		t.Fail()
	}

	//create mock service with ingress but no IP
	s2 := corev1.Service{}
	s2.Name = "test-uid"
	i1 := corev1.LoadBalancerIngress{}
	ing := []corev1.LoadBalancerIngress{i1}
	s2.Status.LoadBalancer.Ingress = ing
	client.CoreV1().Services("test-namespace").Delete(context.Background(), "test-uid", metav1.DeleteOptions{})
	_, _ = client.CoreV1().Services("test-namespace").Create(context.Background(), &s2, metav1.CreateOptions{})

	//Fail on missing service ingress IP
	res, err = server.RetrieveServiceIp(context.Background(), &req)
	if err != nil || res.Status != v1.Status_FAILED || res.Message != "Ip not found!"{
		t.Fail()
	}

	//create mock service with ingress and IP
	s3 := corev1.Service{}
	s3.Name = "test-uid"
	i2 := corev1.LoadBalancerIngress{}
	i2.IP = "10.10.1.1"
	ing2 := []corev1.LoadBalancerIngress{i2}
	s3.Status.LoadBalancer.Ingress = ing2
	client.CoreV1().Services("test-namespace").Delete(context.Background(), "test-uid", metav1.DeleteOptions{})
	_, _ = client.CoreV1().Services("test-namespace").Create(context.Background(), &s3, metav1.CreateOptions{})

	//Pass
	res, err = server.RetrieveServiceIp(context.Background(), &req)
	if res.Status != v1.Status_OK || res.Info != "10.10.1.1" {
		t.Fail()
	}
}

func TestInformationServiceServer_CheckServiceExists(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewInformationServiceServer(client)

	//Fail on API version check
	res, err := server.CheckServiceExists(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.CheckServiceExists(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Fail on loading services
	res, err = server.CheckServiceExists(context.Background(), &req)
	if err == nil || res.Status != v1.Status_FAILED || res.Message != "Service not found!" {
		t.Fail()
	}

	//create mock service without ingress
	s1 := corev1.Service{}
	s1.Name = "test-uid"
	_, _ = client.CoreV1().Services("test-namespace").Create(context.Background(), &s1, metav1.CreateOptions{})

	//Pass
	res, err = server.CheckServiceExists(context.Background(), &req)
	if res.Status != v1.Status_OK {
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
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Pass if already nonexistent
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//Create mock secret
	sec := corev1.Secret{}
	sec.Name = "test-uid-tls"
	_, _ = client.CoreV1().Secrets("test-namespace").Create(context.Background(), &sec, metav1.CreateOptions{})

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
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Pass if already nonexistent
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//Create mock secret
	sec := corev1.Secret{}
	sec.Name = getAuthSecretName("test-uid")
	_, _ = client.CoreV1().Secrets("test-namespace").Create(context.Background() ,&sec, metav1.CreateOptions{})

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

	//Should create new secret
	req := v1.InstanceCredentialsRequest{Api:apiVersion, Instance:&inst, Credentials: &creds}
	res, err = server.CreateOrReplace(context.Background(), &req)
	if res.Status != v1.Status_OK || err != nil {
		t.Fail()
	}

	sec, err := client.CoreV1().Secrets("test-namespace").Get(context.Background(), getAuthSecretName("test-uid"), metav1.GetOptions{})
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
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Should return ok on configmap check if missing
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}

	//create mock configmap
	cm := corev1.ConfigMap{}
	cm.Name = "test-uid"
	_, _ = client.CoreV1().ConfigMaps("test-namespace").Create(context.Background(), &cm, metav1.CreateOptions{})

	//should pass on deleting existing configmap
	res, err = server.DeleteIfExists(context.Background(), &req)
	if err != nil || res.Status != v1.Status_OK {
		t.Fail()
	}
}

func TestPodServiceServer_RetrievePodList(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewPodServiceServer(client)

	//Fail on API version check
	res, err := server.RetrievePodList(context.Background(), &illegal_req)
	if err == nil || res != nil {
		t.Fail()
	}

	//Fail on namespace check
	freq := v1.InstanceRequest{Api:apiVersion, Deployment:&fake_ns_inst}
	res, err = server.RetrievePodList(context.Background(), &freq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Pass
	res, err = server.RetrievePodList(context.Background(), &req)
	if res.Status != v1.Status_OK || len(res.Pods) != 0 {
		t.Fail()
	}

	//create mock pods (2 out of 3 should match the deployment name)
	p1 := corev1.Pod{}
	p1.Name = "test-uid-pod"
	_, _ = client.CoreV1().Pods("test-namespace").Create(context.Background(), &p1, metav1.CreateOptions{})
	p2 := corev1.Pod{}
	p2.Name = "test-uid-pod2"
	_, _ = client.CoreV1().Pods("test-namespace").Create(context.Background(), &p2, metav1.CreateOptions{})
    p3 := corev1.Pod{}
    p3.Name = "test-uid2-pod1"
    _, _ = client.CoreV1().Pods("test-namespace").Create(context.Background(), &p3, metav1.CreateOptions{})

	//Pass
	res, err = server.RetrievePodList(context.Background(), &req)
	if res.Status != v1.Status_OK || len(res.Pods) != 2 || res.Pods[0].Name != "test-uid-pod" {
		t.Fail()
	}
}

func TestPodServiceServer_RetrievePodLogs(t *testing.T) {
	client := testclient.NewSimpleClientset()
	server := NewPodServiceServer(client)

	//Fail on namespace check
	fPodReq := v1.PodRequest{Api:apiVersion, Pod:nil, Deployment:&fake_ns_inst}
	res, err := server.RetrievePodLogs(context.Background(), &fPodReq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock namespace
	ns := corev1.Namespace{}
	ns.Name = "test-namespace"
	_, _ = client.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})

	//Pass
	pod := v1.PodInfo{Name: "test-uid-pod"}
	podReq := v1.PodRequest{Api:apiVersion, Pod:&pod, Deployment:&inst}
	res, err = server.RetrievePodLogs(context.Background(), &podReq)
	if err == nil || res.Status != v1.Status_FAILED {
		t.Fail()
	}

	//create mock pod
	p1 := corev1.Pod{}
	p1.Name = "test-uid-pod"
	_, _ = client.CoreV1().Pods("test-namespace").Create(context.Background(), &p1, metav1.CreateOptions{})

	//Pass
	res, err = server.RetrievePodLogs(context.Background(), &podReq)
    fmt.Printf("Total lines: %d. First line: %s", len(res.Lines), res.Lines[0])
	if err != nil || res.Status != v1.Status_OK || len(res.Lines) != 1 {
		t.Fail()
	}
}

func TestNamespaceServiceServer_CreateNamespace(t *testing.T) {
    client := testclient.NewSimpleClientset()
    server := NewNamespaceServiceServer(client)

	//Fail on API version check
	nsReq := v1.NamespaceRequest{Api:"invalid", Namespace:"ns1", Annotations:nil}
	res, err := server.CreateNamespace(context.Background(), &nsReq)
	if err == nil || res != nil {
		t.Fail()
	}

	nsReq = v1.NamespaceRequest{Api:apiVersion, Namespace:"ns1", Annotations:nil}
    //Pass
    res, err = server.CreateNamespace(context.Background(), &nsReq)
    if err != nil || res.Status != v1.Status_OK {
        t.Fail()
    }
}
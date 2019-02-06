package v1

import (
	"context"
	"encoding/base64"
	"os/exec"
	"github.com/xanzy/go-gitlab"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kube "k8s.io/client-go/kubernetes/typed/core/v1"
	"log"

	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
)

const (
	apiVersion = "v1"
)

type configServiceServer struct {
	kubeAPI kube.CoreV1Interface
	gitAPI *gitlab.Client
}

type basicAuthServiceServer struct {
	kubeAPI kube.CoreV1Interface
}

type certManagerServiceServer struct {
	kubeAPI kube.CoreV1Interface
}

func NewConfigServiceServer(kubeAPI kube.CoreV1Interface, gitAPI *gitlab.Client) v1.ConfigServiceServer {
	return &configServiceServer{kubeAPI: kubeAPI, gitAPI: gitAPI}
}

func NewBasicAuthServiceServer(kubeAPI kube.CoreV1Interface) v1.BasicAuthServiceServer {
	return &basicAuthServiceServer{kubeAPI: kubeAPI}
}

func NewCertManagerServiceServer(kubeAPI kube.CoreV1Interface) v1.CertManagerServiceServer {
	return &certManagerServiceServer{kubeAPI: kubeAPI}
}

func checkAPI(api string) error {
	if len(api) > 0 {
		if apiVersion != api {
			return status.Errorf(codes.Unimplemented,
				"unsupported API version: service implements API version '%s', but asked for '%s'", apiVersion, api)
		}
	}
	return nil
}

//Prepare response
func prepareResponse(status v1.Status, message string) *v1.ServiceResponse {
	return &v1.ServiceResponse{
		Api: apiVersion,
		Status: status,
		Message: message,
	}
}

//Find proper project, given user namespace and instance uid
func (s *configServiceServer) FindGitlabProjectId(api *gitlab.Client, uid string, domain string) (int, error) {
	//Find exact group
	groups, _, err := s.gitAPI.Groups.SearchGroup(domain)
	if len(groups) != 1 || err != nil {
		log.Printf("Found %d groups in domain %s", len(groups), domain)
		log.Print(err)
		return -1, status.Errorf(codes.NotFound, "Gitlab Group for given domain does not exist")
	}

	//List group projects
	projs, _, err := s.gitAPI.Groups.ListGroupProjects(groups[0].ID, nil)
	if err != nil || len(projs) == 0 {
		log.Printf("Group %s is empty or unaccessible", groups[0].Name)
		return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
	}

	//Find our project in group projects list
	for _, proj := range projs {
		if proj.Name == uid {
			return proj.ID, nil
		}
	}

	return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
}

//Parse repository files into kubernetes json data part for patching
func (s *configServiceServer) PrepareDataJsonFromRepository(api *gitlab.Client, repoId int) ([]byte, error) {
	//List files
	tree, _, err := s.gitAPI.Repositories.ListTree(repoId, nil)
	if err != nil {
		log.Print(err)
		return nil, status.Errorf(codes.NotFound, "Cannot find any config files")
	}

	numFiles := len(tree)

	//create helper strings
	mapStart := []byte("{\"binaryData\": {\"")
	mapAfterName := []byte("\": \"")
	mapNextData := []byte("\", \"")
	mapAfterLast := []byte("\"}}")

	//Start parsing
	compiledMap := mapStart
	for i, file := range tree {
		if file.Type != "blob" {
			continue
		}
		opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
		data, _, err := s.gitAPI.RepositoryFiles.GetRawFile(repoId, file.Name, opt)
		if err != nil {
			log.Print(err)
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		compiledMap = append(compiledMap, file.Name...)
		compiledMap = append(compiledMap, mapAfterName...)
		compiledMap = append(compiledMap, base64.StdEncoding.EncodeToString(data)...)

		if numFiles-1 != i { //it's not last element
			compiledMap = append(compiledMap, mapNextData...)
		}
	}
	compiledMap = append(compiledMap, mapAfterLast...)

	return compiledMap, nil
}

//Parse repository files into string:string map for configmap creator
func (s *configServiceServer) PrepareDataMapFromRepository(api *gitlab.Client, repoId int) (map[string][]byte, error) {

	compiledMap := make(map[string][]byte)

	//List files
	tree, _, err := s.gitAPI.Repositories.ListTree(repoId, nil)
	if err != nil {
		log.Print(err)
		return nil, status.Errorf(codes.NotFound, "Cannot find any config files")
	}
	//if len(tree) == 0 {
	//	log.Printf("There are no files to config in repo %d", repoId)
	//	return compiledMap, nil
	//}

	//Start parsing
	for _, file := range tree {
		if file.Type != "blob" {
			continue
		}

		opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
		data, _, err := s.gitAPI.RepositoryFiles.GetRawFile(repoId, file.Name, opt)
		if err != nil {
			log.Print(err)
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		//assign retrieved binary data to newly created configmap
		compiledMap[file.Name] = data
	}

	return compiledMap, nil
}

//Create new configmap
func (s *configServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	proj, err := s.FindGitlabProjectId(s.gitAPI, depl.Uid, depl.Domain)
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Cannot find corresponding gitlab assets"), err
	}

	//check if given k8s namespace exists
	_, err = s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Namespace not found!"), err
	}

	//check if configmap already exists
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Get(depl.Uid, metav1.GetOptions{})
	if err != nil { //Not exists, we create new
		cm := apiv1.ConfigMap{}
		cm.SetName(depl.Uid)
		cm.SetNamespace(depl.Namespace)
		cm.BinaryData, err = s.PrepareDataMapFromRepository(s.gitAPI, proj)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Failed to retrieve data from repository"), err
		}
		_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Create(&cm)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
		}

		return prepareResponse(v1.Status_OK, "ConfigMap created successfully"), nil
	} else { //Already exists, we patch it
		data, err := s.PrepareDataJsonFromRepository(s.gitAPI, proj)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while parsing configuration data"), err
		}

		//patch configmap
		_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Patch(depl.Uid, types.MergePatchType, data)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while patching configmap!"), err
		}

		return prepareResponse(v1.Status_OK, "ConfigMap updated successfully"), nil
	}
}

//Delete configmap for instance
func (s *configServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Namespace not found!"), err
	}

	//check if configmap exist
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Get(depl.Uid, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK,"ConfigMap not exists or is unavailable"), nil
	}

	//delete configmap
	err = s.kubeAPI.ConfigMaps(depl.Namespace).Delete(depl.Uid, &metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing configmap!"), err
	}

	return prepareResponse(v1.Status_OK, "ConfigMap deleted successfully"), nil
}

func (s *basicAuthServiceServer) PrepareSecretDataFromCredentials(credentials *v1.Credentials) (map[string][]byte, error) {
	cmd := exec.Command("htpasswd", "-nb", credentials.User, credentials.Password)
	out, err := cmd.Output()

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to execute htpasswd executable")
	}

	resultMap := make(map[string][]byte)
	resultMap["auth"] = out

	return resultMap, nil
}

func (s *basicAuthServiceServer) PrepareSecretJsonFromCredentials(credentials *v1.Credentials) ([]byte, error) {
	cmd := exec.Command("htpasswd", "-nb", credentials.User, credentials.Password)
	out, err := cmd.Output()

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to execute htpasswd executable")
	}

	result := []byte("{\"data\": {\"auth\": \"")
	result = append(result, out...)
	result = append(result, "\"}}"...)

	return result, nil
}

func getAuthSecretName(uid string) string {
	return uid + "-auth"
}

func (s *basicAuthServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceCredentialsRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Instance

	//check if given k8s namespace exists
	_, err := s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Namespace not found!"), err
	}

	//create secret
	secret := apiv1.Secret{}
	secret.SetNamespace(depl.Namespace)
	secret.SetName(getAuthSecretName(depl.Uid))
	secret.Data, err = s.PrepareSecretDataFromCredentials(req.Credentials)
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while preparing secret!"), err
	}

	//commit secret
	_, err = s.kubeAPI.Secrets(depl.Namespace).Create(&secret)
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while creating secret!"), err
	}

	return prepareResponse(v1.Status_OK, "Secret created successfully"), nil
}

func (s *basicAuthServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Namespace not found!"), err
	}

	secretName := getAuthSecretName(depl.Uid)

	//check if secret exist
	_, err = s.kubeAPI.Secrets(depl.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK,"Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.Secrets(depl.Namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}

	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}

func (s *certManagerServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Namespace not found!"), err
	}

	secretName := depl.Uid + "-tls"

	//check if secret exist
	_, err = s.kubeAPI.Secrets(depl.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK,"Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.Secrets(depl.Namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}

	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}
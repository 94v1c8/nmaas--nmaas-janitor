package v1

import (
	"context"
	"github.com/xanzy/go-gitlab"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kube "k8s.io/client-go/kubernetes/typed/core/v1"
	apiv1 "k8s.io/api/core/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
)

const (
	apiVersion = "v1"
)

type configServiceServer struct {
	kubeAPI kube.CoreV1Interface
	gitAPI *gitlab.Client
}

func NewConfigServiceServer(kubeAPI kube.CoreV1Interface, gitAPI *gitlab.Client) v1.ConfigServiceServer {
	return &configServiceServer{kubeAPI: kubeAPI, gitAPI: gitAPI}
}

func (s *configServiceServer) checkAPI(api string) error {
	if len(api) > 0 {
		if apiVersion != api {
			return status.Errorf(codes.Unimplemented,
				"unsupported API version: service implements API version '%s', but asked for '%s'", apiVersion, api)
		}
	}
	return nil
}

//Prepare response
func (s *configServiceServer) PrepareConfigUpdateResponse(status v1.Status, message string) *v1.ConfigUpdateResponse {
	return &v1.ConfigUpdateResponse{
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
		return -1, status.Errorf(codes.NotFound, "Gitlab Group for given namespace does not exist")
	}

	//List group projects
	projs := groups[0].Projects
	if len(projs) == 0 {
		return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
	}

	//Find our project in group projects list
	found := false
	proj := gitlab.Project{}
	for _, proj := range projs {
		if proj.Name == uid {
			found = true
			break
		}
	}
	if !found {
		return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
	}

	return proj.ID, nil
}

//Parse repository files into kubernetes json data part for patching
func (s *configServiceServer) PrepareDataJsonFromRepository(api *gitlab.Client, repoId int) ([]byte, error) {
	//List files
	tree, _, err := s.gitAPI.Repositories.ListTree(repoId, nil)
	if err != nil || len(tree) == 0 {
		return nil, status.Errorf(codes.NotFound, "Cannot find any config files")
	}

	numFiles := len(tree)

	//create helper strings
	mapStart := []byte("\"data\": {\"")
	mapAfterName := []byte("\": \"")
	mapNextData := []byte("\", \"")
	mapAfterLast := []byte("\"}")

	//Start parsing
	compiledMap := mapStart
	for i, file := range tree {
		if file.Type != "blob" {
			continue
		}

		data, _, err := s.gitAPI.Repositories.RawBlobContent(repoId, file.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		compiledMap = append(compiledMap, file.Name...)
		compiledMap = append(compiledMap, mapAfterName...)
		compiledMap = append(compiledMap, data...)

		if numFiles-1 == i { //it's last element
			compiledMap = append(compiledMap, mapAfterLast...)
		} else {
			compiledMap = append(compiledMap, mapNextData...)
		}
	}

	return compiledMap, nil
}

//Parse repository files into string:string map for configmap creator
func (s *configServiceServer) PrepareDataMapFromRepository(api *gitlab.Client, repoId int) (map[string][]byte, error) {
	//List files
	tree, _, err := s.gitAPI.Repositories.ListTree(repoId, nil)
	if err != nil || len(tree) == 0 {
		return nil, status.Errorf(codes.NotFound, "Cannot find any config files")
	}

	//Start parsing
	compiledMap := make(map[string][]byte)
	for _, file := range tree {
		if file.Type != "blob" {
			continue
		}

		data, _, err := s.gitAPI.Repositories.RawBlobContent(repoId, file.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		//assign retrieved binary data to newly created configmap
		compiledMap[file.Name] = data
	}

	return compiledMap, nil
}

//Create new configmap
func (s *configServiceServer) Create(ctx context.Context, req *v1.ConfigUpdateRequest) (*v1.ConfigUpdateResponse, error) {
	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	proj, err := s.FindGitlabProjectId(s.gitAPI, depl.Uid, depl.Domain)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Cannot find corresponding gitlab assets"), err
	}


	cm := apiv1.ConfigMap{}
	cm.SetName(depl.Uid)
	cm.SetNamespace(depl.Namespace)
	cm.BinaryData, err = s.PrepareDataMapFromRepository(s.gitAPI, proj)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Failed to retrieve data from repository"), err
	}

	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Create(&cm)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
	}


	return s.PrepareConfigUpdateResponse(v1.Status_OK, "ConfigMap created successfully"), nil
}

// Update configmap for instance
func (s *configServiceServer) Update(ctx context.Context, req *v1.ConfigUpdateRequest) (*v1.ConfigUpdateResponse, error) {
	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	proj, err := s.FindGitlabProjectId(s.gitAPI, depl.Uid, depl.Domain)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Cannot find corresponding gitlab assets"), err
	}

	data, err := s.PrepareDataJsonFromRepository(s.gitAPI, proj)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Error while parsing configuration data"), err
	}


	//check if given k8s namespace exists
	_, err = s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Namespace not found!"), err
	}
	
	//check if updated configmap exist
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Get(depl.Uid, metav1.GetOptions{})
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED,"ConfigMap not found or is unavailable"), err
	}

	//patch configmap
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Patch(depl.Uid, types.JSONPatchType, data)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Error while patching configmap!"), err
	}

	return s.PrepareConfigUpdateResponse(v1.Status_OK, "ConfigMap updated successfully"), nil
}
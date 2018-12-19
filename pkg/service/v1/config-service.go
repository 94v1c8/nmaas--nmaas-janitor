package v1

import (
	"context"
	"github.com/xanzy/go-gitlab"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kube "k8s.io/client-go/kubernetes/typed/core/v1"

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

func (s *configServiceServer) PrepareConfigUpdateResponse(status v1.Status, message string) *v1.ConfigUpdateResponse {
	return &v1.ConfigUpdateResponse{
		Api: apiVersion,
		Status: status,
		Message: "Gitlab Group for given namespace does not exist",
	}
}

// Update configmap for instance
func (s *configServiceServer) Update(ctx context.Context, req *v1.ConfigUpdateRequest) (*v1.ConfigUpdateResponse, error) {
	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//Find exact group
	groups, _, err := s.gitAPI.Groups.SearchGroup(depl.Namespace)
	if len(groups) != 1 || err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Gitlab Group for given namespace does not exist"), nil
	}

	//List group projects
	projs := groups[0].Projects
	if len(projs) == 0 {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Project containing config not found on Gitlab"), nil
	}

	//Find our project in group projects list
	found := false
	proj := gitlab.Project{}
	for _, proj := range projs {
		if proj.Name == depl.Uid {
			found = true
			break
		}
	}
	if !found {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Project containing config not found on Gitlab"), nil
	}

	//List files
	tree, _, err := s.gitAPI.Repositories.ListTree(proj.ID, nil)
	if err != nil || len(tree) == 0 {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Cannot find any config files"), nil
	}

	//create patch file using tree
	mapStart := []byte("\"data\": {\"")
	mapAfterName := []byte("\": \"")
	mapNextData := []byte("\", \"")
	mapAfterLast := []byte("\"}")
	compiledMap := mapStart

	numFiles := len(tree)
	for i, file := range tree {
		if file.Type != "blob" {
			continue
		}

		data, _, err := s.gitAPI.Repositories.RawBlobContent(proj.ID, file.ID)
		if err != nil {
			return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Error while reading file from Gitlab!"), nil
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

	//check if given k8s namespace exists
	_, err = s.kubeAPI.Namespaces().Get(depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Namespace not found!"), nil
	}
	
	//check if updated configmap exist
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Get(depl.Uid, metav1.GetOptions{})
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED,"ConfigMap not found or is unavailable"), nil
	}

	//patch configmap
	_, err = s.kubeAPI.ConfigMaps(depl.Namespace).Patch(depl.Uid, types.JSONPatchType, compiledMap)
	if err != nil {
		return s.PrepareConfigUpdateResponse(v1.Status_FAILED, "Error while patching configmap!"), nil
	}

	return s.PrepareConfigUpdateResponse(v1.Status_OK, "ConfigMap updated successfully"), nil
}
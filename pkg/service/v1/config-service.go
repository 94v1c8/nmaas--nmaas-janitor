package v1

import (
	"context"
	"encoding/base64"
	"github.com/xanzy/go-gitlab"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"log"
	"math/rand"
	"strings"
	"time"
	"fmt"

	v1 "bitbucket.software.geant.org/projects/NMAAS/repos/nmaas-janitor/pkg/api/v1"
	"github.com/johnaoss/htpasswd/apr1"
)

const (
	apiVersion = "v1"
	namespaceNotFound = "Namespace not found"
)

type configServiceServer struct {
	kubeAPI kubernetes.Interface
	gitAPI *gitlab.Client
}

type basicAuthServiceServer struct {
	kubeAPI kubernetes.Interface
}

type certManagerServiceServer struct {
	kubeAPI kubernetes.Interface
}

type readinessServiceServer struct {
	kubeAPI kubernetes.Interface
}

type informationServiceServer struct {
	kubeAPI kubernetes.Interface
}

func NewConfigServiceServer(kubeAPI kubernetes.Interface, gitAPI *gitlab.Client) v1.ConfigServiceServer {
	return &configServiceServer{kubeAPI: kubeAPI, gitAPI: gitAPI}
}

func NewBasicAuthServiceServer(kubeAPI kubernetes.Interface) v1.BasicAuthServiceServer {
	return &basicAuthServiceServer{kubeAPI: kubeAPI}
}

func NewCertManagerServiceServer(kubeAPI kubernetes.Interface) v1.CertManagerServiceServer {
	return &certManagerServiceServer{kubeAPI: kubeAPI}
}

func NewReadinessServiceServer(kubeAPI kubernetes.Interface) v1.ReadinessServiceServer {
	return &readinessServiceServer{kubeAPI: kubeAPI}
}

func NewInformationServiceServer(kubeAPI kubernetes.Interface) v1.InformationServiceServer {
	return &informationServiceServer{kubeAPI: kubeAPI}
}

func log(message string) {
    log.Printf("%s : %s", time.Now(), message)
}

func checkAPI(api string, current string) error {
	if len(api) > 0 && current != api {
		return status.Errorf(codes.Unimplemented,
			"unsupported API version: service implements API version '%s', but asked for '%s'", apiVersion, api)
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

//Prepare info response
func prepareInfoResponse(status v1.Status, message string, info string) *v1.InfoServiceResponse {
	return &v1.InfoServiceResponse{
		Api: apiVersion,
		Status: status,
		Message: message,
		Info: info,
	}
}

//Find proper project, given user namespace and instance uid
func (s *configServiceServer) FindGitlabProjectId(api *gitlab.Client, uid string, domain string) (int, error) {
	//Find exact group
	log.Printf("Searching for GitLab Group by domain %s", domain)
	groups, _, err := api.Groups.SearchGroup(domain)
	if len(groups) != 1 || err != nil {
		log.Printf("Found %d groups in domain %s", len(groups), domain)
		log.Print(err)
		return -1, status.Errorf(codes.NotFound, "Gitlab Group for given domain does not exist")
	}

	//List group projects
	log.Printf("Searching for GitLab Projects within Group %d / %s", groups[0].ID, groups[0].Name)
	projs, _, err := api.Groups.ListGroupProjects(groups[0].ID, nil)
	if err != nil || len(projs) == 0 {
		log.Printf("Group %s is empty or inaccessible", groups[0].Name)
		return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
	}

	//Find our project in group projects list
    log.Printf("Found %d projects and looking for %s", len(projs), uid)
	for _, proj := range projs {
		if proj.Name == uid {
			return proj.ID, nil
		}
	}

	return -1, status.Errorf(codes.NotFound, "Project containing config not found on Gitlab")
}

//Parse repository files into string:string map for configmap creator
func (s *configServiceServer) PrepareDataMapFromRepository(api *gitlab.Client, repoId int) (map[string]map[string]string, error) {

	var compiledMap = map[string]map[string]string{}

	//Processing files in root directory
	log.Print("Processing files in root directory")

	rootTree, _, err := api.Repositories.ListTree(repoId, nil)
	if err != nil {
		log.Print(err)
	}

	directoryMap := make(map[string]string)

	//Start parsing
	for _, file := range rootTree {

		if file.Type != "blob" {
			continue
		}
		log.Printf("Processing new file from repository (name: %s, path: %s)", file.Name, file.Path)

		opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
		fileContent, _, err := api.RepositoryFiles.GetRawFile(repoId, file.Path, opt)
		if err != nil {
			log.Print(err)
			return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
		}

		//assign retrieved binary data to newly created configmap
		directoryMap[file.Name] = string(fileContent)
	}

	compiledMap[""] = directoryMap

	//List files recursively
	opt := &gitlab.ListTreeOptions{Recursive: gitlab.Bool(true)}
	treeRec, _, err := api.Repositories.ListTree(repoId, opt)

	//List directories (apart from root)
	for _, directory := range treeRec {

		if directory.Type == "tree" {

			log.Printf("Processing new directory from repository (name: %s, path: %s)", directory.Name, directory.Path)

			opt := &gitlab.ListTreeOptions{Path: gitlab.String(directory.Path), Recursive: gitlab.Bool(true)}
			dirTree, _, err := api.Repositories.ListTree(repoId, opt)
			if err != nil {
				log.Print(err)
			}

			directoryMap := make(map[string]string)

			//Start parsing
			for _, file := range dirTree {

				if file.Type != "blob" {
					continue
				}

				log.Printf("Processing new file from repository (name: %s, path: %s)", file.Name, file.Path)

				opt := &gitlab.GetRawFileOptions{Ref: gitlab.String("master")}
				fileContent, _, err := api.RepositoryFiles.GetRawFile(repoId, file.Path, opt)
				if err != nil {
					log.Print(err)
					return nil, status.Errorf(codes.Internal, "Error while reading file from Gitlab!")
				}

				//assign retrieved binary data to newly created configmap
				directoryMap[file.Name] = string(fileContent)
			}

			compiledMap[directory.Name] = directoryMap
		}
	}

	return compiledMap, nil
}

//Create new configmap
func (s *configServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	proj, err := s.FindGitlabProjectId(s.gitAPI, depl.Uid, depl.Domain)
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Cannot find corresponding gitlab assets"), err
	}

	//check if given k8s namespace exists
	_, err = s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		ns := apiv1.Namespace{}
		ns.Name = depl.Namespace
		_, err = s.kubeAPI.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
		}
	}

	var repo = map[string]map[string]string{}

	repo, err = s.PrepareDataMapFromRepository(s.gitAPI, proj)
	if err != nil {
		log.Print("Error occurred while retriving content of the Git repository. Will not create any ConfigMap")
		return prepareResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
	}

	for directory, files := range repo {

		cm := apiv1.ConfigMap{}
		if len(directory) > 0 {
			cm.SetName(depl.Uid + "-" + directory)
		} else {
			cm.SetName(depl.Uid)
		}
		cm.SetNamespace(depl.Namespace)
		cm.Data = files

		//check if configmap already exists
		_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})

		if err != nil { //Not exists, we create new

			_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Create(ctx, &cm, metav1.CreateOptions{})
			if err != nil {
				return prepareResponse(v1.Status_FAILED, "Failed to create ConfigMap"), err
			}

		} else { //Already exists, we update it

			_, err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Update(ctx, &cm, metav1.UpdateOptions{})
			if err != nil {
				return prepareResponse(v1.Status_FAILED, "Error while updating configmap!"), err
			}
		}
	}

	return prepareResponse(v1.Status_OK, "ConfigMap created successfully"), nil
}

//Delete all config maps for instance
func (s *configServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	//retrieve list of configmaps in namespace
	configMaps, err := s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK, "Could not retrieve list of ConfigMaps in namespace"), nil
	}

    for _, configmap := range configMaps.Items {
		//check if configmap belongs to instance
		if configmap.Name == depl.Uid || strings.Contains(configmap.Name, depl.Uid + "-") {
			//delete configmap
			log.Printf("Deleting ConfigMap named %s", configmap.Name)
			err = s.kubeAPI.CoreV1().ConfigMaps(depl.Namespace).Delete(ctx, configmap.Name, metav1.DeleteOptions{})
			if err != nil {
				log.Printf("Error occurred while deleting ConfigMap %s", configmap.Name)
			}
		}
	}

	return prepareResponse(v1.Status_OK, "ConfigMaps deleted successfully"), nil
}

func randomString(l int) string {
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
		bytes[i] = byte(65 + rand.Intn(90-65))
	}
	return string(bytes)
}

func aprHashCredentials(user string, password string) (string, error) {
	out, err := apr1.Hash(password, randomString(8))

	if err != nil {
		return "", status.Errorf(codes.Internal, "Failed to execute apr hashing")
	}

	return user + ":" + out, nil
}

func (s *basicAuthServiceServer) PrepareSecretDataFromCredentials(credentials *v1.Credentials) (map[string][]byte, error) {
	hash, err := aprHashCredentials(credentials.User, credentials.Password)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string][]byte)
	resultMap["auth"] = []byte(hash)

	return resultMap, nil
}

func (s *basicAuthServiceServer) PrepareSecretJsonFromCredentials(credentials *v1.Credentials) ([]byte, error) {
	hash, err := aprHashCredentials(credentials.User, credentials.Password)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to execute htpasswd executable")
	}

	result := []byte("{\"data\": {\"auth\": \"")
	result = append(result, base64.StdEncoding.EncodeToString([]byte(hash))...)
	result = append(result, "\"}}"...)

	return result, nil
}

func getAuthSecretName(uid string) string {
	return uid + "-auth"
}

func (s *basicAuthServiceServer) CreateOrReplace(ctx context.Context, req *v1.InstanceCredentialsRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Instance

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil{
		ns := apiv1.Namespace{}
		ns.Name = depl.Namespace
		_, err = s.kubeAPI.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
		}
	}

	secretName := getAuthSecretName(depl.Uid)

	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	//Secret does not exist, we have to create it
	if err != nil {
		//create secret
		secret := apiv1.Secret{}
		secret.SetNamespace(depl.Namespace)
		secret.SetName(secretName)
		secret.Data, err = s.PrepareSecretDataFromCredentials(req.Credentials)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while preparing secret!"), err
		}

		//commit secret
		_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Create(ctx, &secret, metav1.CreateOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while creating secret!"), err
		}

		return prepareResponse(v1.Status_OK, "Secret created successfully"), nil
	} else {
		patch, err := s.PrepareSecretJsonFromCredentials(req.Credentials)
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while parsing configuration data"), err
		}

		//patch secret
		_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Patch(ctx, secretName, types.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return prepareResponse(v1.Status_FAILED, "Error while patching secret!"), err
		}

		return prepareResponse(v1.Status_OK, "Secret updated successfully"), nil
	}
}

func (s *basicAuthServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	secretName := getAuthSecretName(depl.Uid)

	//check if secret exist
	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK,"Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}

	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}

func (s *certManagerServiceServer) DeleteIfExists(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	secretName := depl.Uid + "-tls"

	//check if secret exist
	_, err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_OK,"Secret does not exist"), nil
	}

	//delete secret
	err = s.kubeAPI.CoreV1().Secrets(depl.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, "Error while removing secret!"), err
	}

	return prepareResponse(v1.Status_OK, "Secret deleted successfully"), nil
}

func (s *readinessServiceServer) CheckIfReady(ctx context.Context, req *v1.InstanceRequest) (*v1.ServiceResponse, error) {
	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment
	log(fmt.Sprintf("> Check if deployment:%s ready in namespace:%s", depl.Uid, depl.Namespace))

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareResponse(v1.Status_FAILED, namespaceNotFound), err
	}

	log("looking for deployment and checking its status")
	dep, err := s.kubeAPI.AppsV1().Deployments(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
		log("deployment not found, looking for statefulset and checking its status")
		sts, err2 := s.kubeAPI.AppsV1().StatefulSets(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
		if err2 != nil {
			log("statefulset not found as well")
			return prepareResponse(v1.Status_FAILED, "Neither Deployment nor StatefulSet found!"), err2
		} else {
			log("statefulset found, verifying status")
			if *sts.Spec.Replicas == sts.Status.ReadyReplicas {
		        log("ready")
				return prepareResponse(v1.Status_OK, "StatefulSet is ready"), nil
			}
			log("not yet ready")
			return prepareResponse(v1.Status_PENDING, "Waiting for statefulset"), nil
		}
	} else {
		log("deployment found, verifying status")
		if *dep.Spec.Replicas == dep.Status.ReadyReplicas {
		    log("ready")
			return prepareResponse(v1.Status_OK, "Deployment is ready"), nil
		}
		log("not yet ready")
		return prepareResponse(v1.Status_PENDING, "Waiting for deployment"), nil
	}

}

func (s *informationServiceServer) RetrieveServiceIp(ctx context.Context, req *v1.InstanceRequest) (*v1.InfoServiceResponse, error) {

	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment
	log.Printf("> Retrieving IP for service:%s in namespace:%s", depl.Uid, depl.Namespace)

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, namespaceNotFound, ""), err
	}

	app, err := s.kubeAPI.CoreV1().Services(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
	    log.Printf("Service not found")
		return prepareInfoResponse(v1.Status_FAILED, "Service not found!", ""), err
	}

	if len(app.Status.LoadBalancer.Ingress) > 0 {
		log.Printf("Found %d loadbalancer ingresse(s)", len(app.Status.LoadBalancer.Ingress))

		ip := app.Status.LoadBalancer.Ingress[0].IP

		if ip != "" {
			log.Printf("Found IP address. Will return %s", ip)
			return prepareInfoResponse(v1.Status_OK, "", ip), err
		} else {
			log.Printf("IP adress not found")
			return prepareInfoResponse(v1.Status_FAILED, "Ip not found!", ""), err
		}

	} else {
		log.Printf("No loadbalancer ingresses found")
		return prepareInfoResponse(v1.Status_FAILED, "Service ingress not found!", ""), err
	}

}

func (s *informationServiceServer) CheckServiceExists(ctx context.Context, req *v1.InstanceRequest) (*v1.InfoServiceResponse, error) {

	log.Printf("Entered CheckServiceExists method")

	// check if the API version requested by client is supported by server
	if err := checkAPI(req.Api, apiVersion); err != nil {
		return nil, err
	}

	depl := req.Deployment

	//check if given k8s namespace exists
	_, err := s.kubeAPI.CoreV1().Namespaces().Get(ctx, depl.Namespace, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, namespaceNotFound, ""), err
	}

	log.Printf("About to read service %s details from namespace %s", depl.Uid, depl.Namespace)

	ser, err := s.kubeAPI.CoreV1().Services(depl.Namespace).Get(ctx, depl.Uid, metav1.GetOptions{})
	if err != nil {
		return prepareInfoResponse(v1.Status_FAILED, "Service not found!", ""), err
	}
	
	return prepareInfoResponse(v1.Status_OK, "", ser.Name), err

}

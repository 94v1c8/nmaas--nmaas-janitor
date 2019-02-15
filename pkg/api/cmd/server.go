package cmd

import (
	"context"
	"flag"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"github.com/xanzy/go-gitlab"
	"log"

	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/protocol/grpc"
	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/service/v1"
)

// Config is configuration for Server
type Config struct {
	GRPCPort string
	GitlabToken string
	GitlabURL string
}

// RunServer runs gRPC server and HTTP gateway
func RunServer() error {
	ctx := context.Background()

	// get configuration
	var cfg Config
	flag.StringVar(&cfg.GRPCPort, "port", "", "gRPC port to bind")
	flag.StringVar(&cfg.GitlabToken, "token", "", "Gitlab token")
	flag.StringVar(&cfg.GitlabURL, "url", "", "Gitlab API URL")
	flag.Parse()

	if len(cfg.GRPCPort) == 0 {
		return fmt.Errorf("invalid TCP port for gRPC server: '%s'", cfg.GRPCPort)
	}

	//Initialize kubernetes API
	config, err := rest.InClusterConfig()
    if err != nil {
        log.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatal(err)
    }
	kubeAPI := clientset.CoreV1()
	
	//Initialize Gitlab API
	gitAPI := gitlab.NewClient(nil, cfg.GitlabToken)
	err = gitAPI.SetBaseURL(cfg.GitlabURL)
	if err != nil {
		log.Fatal(err)
	}

	confAPI := v1.NewConfigServiceServer(kubeAPI, gitAPI)
	authAPI := v1.NewBasicAuthServiceServer(kubeAPI)
	certAPI := v1.NewCertManagerServiceServer(kubeAPI)
	readyAPI := v1.NewReadinessServiceServer(kubeAPI)

	return grpc.RunServer(ctx, confAPI, authAPI, certAPI, readyAPI, cfg.GRPCPort)
}
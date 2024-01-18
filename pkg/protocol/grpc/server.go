package grpc

import (
	"context"
	"net"
	"os"
	"os/signal"

	"google.golang.org/grpc"

	v1 "bitbucket.software.geant.org/projects/NMAAS/repos/nmaas-janitor/pkg/api/v1"
)

func RunServer(ctx context.Context,
               confAPI v1.ConfigServiceServer,
               authAPI v1.BasicAuthServiceServer,
               certAPI v1.CertManagerServiceServer,
               readyAPI v1.ReadinessServiceServer,
               infoAPI v1.InformationServiceServer,
               podAPI v1.PodServiceServer,
               port string) error {
	listen, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// register services
	server := grpc.NewServer()
	v1.RegisterConfigServiceServer(server, confAPI)
	v1.RegisterBasicAuthServiceServer(server, authAPI)
	v1.RegisterCertManagerServiceServer(server, certAPI)
	v1.RegisterReadinessServiceServer(server, readyAPI)
	v1.RegisterInformationServiceServer(server, infoAPI)
	v1.RegisterPodServiceServer(server, podAPI)

	// graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			server.GracefulStop()

			<-ctx.Done()
		}
	}()

	// start gRPC server
	return server.Serve(listen)
}

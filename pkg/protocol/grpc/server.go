package grpc

import (
	"context"
	"net"
	"os"
	"os/signal"

	"google.golang.org/grpc"

	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
)

func RunServer(ctx context.Context, confAPI v1.ConfigServiceServer, authAPI v1.BasicAuthServiceServer, certAPI v1.CertManagerServiceServer, port string) error {
	listen, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// register services
	server := grpc.NewServer()
	v1.RegisterConfigServiceServer(server, confAPI)
	v1.RegisterBasicAuthServiceServer(server, authAPI)
	v1.RegisterCertManagerServiceServer(server, certAPI)

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
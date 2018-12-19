package grpc

import (
	"context"
	"net"
	"os"
	"os/signal"

	"google.golang.org/grpc"

	"code.geant.net/stash/scm/nmaas/nmaas-janitor/pkg/api/v1"
)

func RunServer(ctx context.Context, v1API v1.ConfigServiceServer, port string) error {
	listen, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// register service
	server := grpc.NewServer()
	v1.RegisterConfigServiceServer(server, v1API)

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
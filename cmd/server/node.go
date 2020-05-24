package main

import (
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"github.com/zhigui-projects/go-hotstuff/network"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var replicaId int64
var listenAddress string
var listenPort string
var tlsEnabled bool

var nodeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the hotstuff node.",
	Long:  `Start a hotstuff node that interacts with the consensus network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return serve(args)
	},
}

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.Int64VarP(&replicaId, "replicaId", "", 1, "hotstuff node replica id")
	flags.StringVarP(&listenAddress, "listenAddress", "", "127.0.0.1", "hotstuff node server listen address")
	flags.StringVarP(&listenPort, "port", "p", "8000", "hotstuff node listen port")
	flags.BoolVar(&tlsEnabled, "tls", false, "Use TLS when communicating with the hotstuff node endpoint")
	return nodeStartCmd
}

func serve(args []string) error {
	grpcServer, err := network.NewGrpcServer(net.JoinHostPort(listenAddress, listenPort), nil)
	if err != nil {
		logger.Error("Failed to new grpc server", "error", err)
		return err
	}

	server := network.NewABServer()
	pb.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
	logger.Info("Hotstuff node started, beginning to serve requests", "replicaId", replicaId, "serverAddress", net.JoinHostPort(listenAddress, listenPort))
	grpcServer.Start()
	return nil
}

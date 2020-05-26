package main

import (
	"context"
	"fmt"
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"os"

	"github.com/spf13/cobra"
	"github.com/zhigui-projects/go-hotstuff/common/log"
)

var logger = log.GetLogger("module", "cmd.server")

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{Use: "hotstuff-server"}

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

func main() {
	mainCmd.AddCommand(startCmd())
	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

var nodes = []*consensus.NodeInfo{
	{
		Id:      1,
		Addr:    "127.0.0.1:8000",
		TlsOpts: nil,
	},
	{
		Id:      2,
		Addr:    "127.0.0.1:8001",
		TlsOpts: nil,
	},
}

func serve(args []string) error {
	hsb := consensus.NewHotStuffBase(consensus.ReplicaID(replicaId), nodes, nil, nil)
	hsb.Start(context.Background())
	return nil
}

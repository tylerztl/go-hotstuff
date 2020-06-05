package main

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zhigui-projects/go-hotstuff/common/crypto"
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/pacemaker"
)

const (
	node0 = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgPEKtXy8Y2EibCmkU
vxtjkwLfMbLDW8AMyoAMuqo19LmhRANCAAQcnT3TXcSLBeAjmCiqsdxgLks4DL4X
JuWHVgwpSt29P576HvmISXR2Yt6ANzS31wEN6eZZjEd47e6s1fqZW4mI
-----END PRIVATE KEY-----
`
	node1 = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgPpKflh9pkNFSsY8c
97MUPKl6HFLboKNjGI/AKtJ9RWOhRANCAATB9/poI01ioHqV3i51QI3gOC5sQjhU
9dEr8nLs5RzCboPCmniL/b4QCAPvpFBMLQtcxm/P/FNJYyibPk50BonF
-----END PRIVATE KEY-----
`
	node2 = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgb9DMT9J1IKyO3+Wo
Bx7SRYfsrJEWDc4+mrUxlbIaEoWhRANCAAQ/8EuFKwymyu2Ge6W8OTdVhKu7JNOh
JJxfhUoeCQKSq1BhTI/7rVa+8LHchHG0SQm6xDpP/xpR3c7GkFgg72bm
-----END PRIVATE KEY-----
`
	node3 = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgu7eHr3dkU/N/fsM1
zAGN4EIqg2IeglBjXQKe3neICwChRANCAARmsvIihcHs6eGHmbx+pa+1K9vOV+OC
2LTobiPYjEBdXf/vGbsF4y1pS4VuwZYWxkfm2eqrajGDjnDIywly7tVe
-----END PRIVATE KEY-----
`
)

var replica0PK ecdsa.PrivateKey
var replica1PK ecdsa.PrivateKey
var replica2PK ecdsa.PrivateKey
var replica3PK ecdsa.PrivateKey

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
	PersistentPreRun: initCmd,
}

func initCmd(cmd *cobra.Command, args []string) {
	pk0, err := crypto.ParsePrivateKey([]byte(node0))
	if err != nil {
		panic(err)
	}

	replica0PK = *pk0.(*ecdsa.PrivateKey)

	pk1, err := crypto.ParsePrivateKey([]byte(node1))
	if err != nil {
		panic(err)
	}
	replica1PK = *pk1.(*ecdsa.PrivateKey)

	pk2, err := crypto.ParsePrivateKey([]byte(node2))
	if err != nil {
		panic(err)
	}
	replica2PK = *pk2.(*ecdsa.PrivateKey)

	pk3, err := crypto.ParsePrivateKey([]byte(node3))
	if err != nil {
		panic(err)
	}
	replica3PK = *pk3.(*ecdsa.PrivateKey)
}

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.Int64VarP(&replicaId, "replicaId", "", 0, "hotstuff node replica id")
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
		Id:      0,
		Addr:    "127.0.0.1:8000",
		TlsOpts: nil,
	},
	{
		Id:      1,
		Addr:    "127.0.0.1:8001",
		TlsOpts: nil,
	},
	{
		Id:      2,
		Addr:    "127.0.0.1:8002",
		TlsOpts: nil,
	},
	{
		Id:      3,
		Addr:    "127.0.0.1:8003",
		TlsOpts: nil,
	},
}

var replicas = &consensus.ReplicaConf{
	QuorumSize: 3,
	Replicas: map[consensus.ReplicaID]*consensus.ReplicaInfo{
		0: {ID: 0, Verifier: &crypto.ECDSAVerifier{Pub: &replica0PK.PublicKey}},
		1: {ID: 1, Verifier: &crypto.ECDSAVerifier{Pub: &replica1PK.PublicKey}},
		2: {ID: 2, Verifier: &crypto.ECDSAVerifier{Pub: &replica2PK.PublicKey}},
		3: {ID: 3, Verifier: &crypto.ECDSAVerifier{Pub: &replica3PK.PublicKey}},
	},
}

func serve(args []string) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var signer *ecdsa.PrivateKey
	if replicaId == 0 {
		signer = &replica0PK
	} else if replicaId == 1 {
		signer = &replica1PK
	} else if replicaId == 2 {
		signer = &replica2PK
	} else if replicaId == 3 {
		signer = &replica3PK
	}
	hsb := consensus.NewHotStuffBase(consensus.ReplicaID(replicaId), nodes, &crypto.ECDSASigner{Pri: signer}, replicas)
	rr := pacemaker.NewRoundRobinPM(hsb)
	rr.Run()

	<-signals
	return nil
}

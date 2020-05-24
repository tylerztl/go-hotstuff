package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/zhigui-projects/go-hotstuff/common/log"
)

var logger = log.GetLogger("path", "cmd.server")

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{Use: "hotstuff-server"}

func main() {
	mainCmd.AddCommand(startCmd())
	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

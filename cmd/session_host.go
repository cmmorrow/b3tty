package cmd

import (
	"os"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
)

// sessionHostCmd is the process that owns one session's pty behind a Unix
// socket (src.RunSessionHostProcess). The server starts it with
// src.SpawnSessionHost, which writes the session's spec on its stdin; it is
// not meant to be run by hand, so it is hidden from help.
var sessionHostCmd = &cobra.Command{
	Use:    "session-host",
	Short:  "Run a session host (started by the b3tty server)",
	Hidden: true,
	Args:   cobra.NoArgs,
	// Replaces rootCmd's PersistentPreRun, so no themes are loaded.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(src.RunSessionHostProcess())
	},
}

// isSessionHostInvocation reports whether this process is a session host,
// so initConfig can skip reading the config file and plugin store.
func isSessionHostInvocation() bool {
	c, _, err := rootCmd.Find(os.Args[1:])
	return err == nil && c == sessionHostCmd
}

func init() {
	rootCmd.AddCommand(sessionHostCmd)
}

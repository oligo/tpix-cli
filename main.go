package main

import (
	"github.com/oligo/tpix-cli/config"
	"github.com/spf13/cobra"
)

var (
	rootCmd    = cobra.Command{
		Use:   "tpix-cli",
		Short: "A tpix command line client used to manage Typst packages",
	}
)

func main() {
	// Load config on startup
	config.Load()

	//rootCmd.PersistentFlags().StringVar(&tpixServer, "server", tpixServer, "TPIX server URL")

	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(searchPkgCmd())
	rootCmd.AddCommand(getPkgCmd())
	rootCmd.AddCommand(queryPkgCmd())
	rootCmd.AddCommand(listCachedCmd())
	rootCmd.AddCommand(removeCachedCmd())
	rootCmd.AddCommand(bundleCmd())
	rootCmd.AddCommand(pushCmd())

	rootCmd.Execute()
}

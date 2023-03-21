/*
Copyright © 2022 Metal toolbox authors <>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/spf13/cobra"
)

var (
	logLevel string
	cfgFile  string

	// storeKind is inventory store name - serverservice
	storeKind string

	// outputStdout when set causes alloy to write the collected data to stdout
	outputStdout bool

	enableProfiling bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "alloy",
	Short: "server inventory and bios configuration collector",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Read in env vars with appName as prefix
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "configuration file")
	rootCmd.PersistentFlags().StringVar(&storeKind, "store", "mock", "The inventory store kind (serverservice, csv)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "set logging level - debug, trace")
	rootCmd.PersistentFlags().BoolVarP(&outputStdout, "output-stdout", "", false, "Output collected data to STDOUT instead of the store")
	rootCmd.PersistentFlags().BoolVarP(&enableProfiling, "enable-pprof", "", false, "Enable profiling endpoint at: "+model.ProfilingEndpoint)
}

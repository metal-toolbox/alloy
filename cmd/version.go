package cmd

import (
	"fmt"

	"github.com/metal-toolbox/alloy/internal/version"
	"github.com/peterbourgon/ff/v3/ffcli"
	"golang.org/x/net/context"
)

type versionCmd struct {
	rootCmd *rootCmd
}

func newVersionCmd(rootCmd *rootCmd) *ffcli.Command {
	c := &versionCmd{rootCmd: rootCmd}

	return &ffcli.Command{
		Name:       "version",
		ShortUsage: "version",
		ShortHelp:  "Print Alloy version along with dependency - ironlib, bmclib version information.",
		Exec:       c.Exec,
	}
}

func (c *versionCmd) Exec(ctx context.Context, _ []string) error {
	fmt.Printf(
		"commit: %s\nbranch: %s\ngit summary: %s\nbuildDate: %s\nversion: %s\nGo version: %s\nironlib version: %s\nbmclib version: %s\n",
		version.GitCommit, version.GitBranch, version.GitSummary, version.BuildDate, version.AppVersion, version.GoVersion, version.IronlibVersion, version.BmclibVersion)

	return nil
}

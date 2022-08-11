package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"

	"github.com/peterbourgon/ff/v3/ffcli"
	"golang.org/x/net/context"

	"github.com/equinix-labs/otel-init-go/otelinit"
)

var (
	errParseCLIParam = errors.New("parameter parse failed")
)

// Run is the main command entry point, all sub commands are registered here
func Run() error {
	var (
		cmd, cfg     = newRootCmd()
		outOfBandCmd = newOutOfBandCmd(cfg)
		inbandCmd    = newInbandCmd(cfg)
	)

	cmd.Subcommands = append(cmd.Subcommands, outOfBandCmd, inbandCmd)

	if err := cmd.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error in cli parse: %v\n", err)
		os.Exit(1)
	}

	// setup otel telemetry
	ctx := context.Background()
	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "alloy")

	defer otelShutdown(ctx)

	return cmd.Run(ctx)
}

// rootCmd is the cli root command instance, it holds attributes available to subcommands
type rootCmd struct {
	// cfgFile is the configuration file
	cfgFile string
	// publisherKind is where collected inventory is published
	// stdout OR csv
	publisherKind string

	// flag sets trace log level
	trace bool
	// flag sets debug log level
	debug bool
	// flag enables pprof endpoint on localhost:9091
	pprof bool
}

func (c *rootCmd) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.debug, "debug", false, "Set logging to debug level.")
	fs.BoolVar(&c.trace, "trace", false, "Set logging to trace level.")
	fs.BoolVar(&c.pprof, "profile", false, "Enable performance profile endpoint.")
	fs.StringVar(&c.cfgFile, "config-file", "", "Alloy config file")
	fs.StringVar(&c.publisherKind, "publish-target", "stdout", "Publish collected inventory to [serverService|stdout]")
}

func (c *rootCmd) Exec(context.Context, []string) error {
	return flag.ErrHelp
}

func (c *rootCmd) LogLevel() int {
	switch {
	case c.debug:
		return model.LogLevelDebug
	case c.trace:
		return model.LogLevelTrace
	default:
		return model.LogLevelInfo
	}
}

func newRootCmd() (*ffcli.Command, *rootCmd) {
	var c rootCmd

	fs := flag.NewFlagSet("alloy", flag.ExitOnError)
	c.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       "alloy",
		ShortHelp:  "alloy collects device inventory attributes",
		ShortUsage: "alloy [inband|outofband] [flags]",
		FlagSet:    fs,
		Exec:       c.Exec,
	}, &c
}

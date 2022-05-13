package main

import (
	"log"
	"os"

	"github.com/urfave/cli"
)

func run() {
	collector := &collector{}

	app := cli.NewApp()

	app.Commands = []cli.Command{
		{
			Name:  "inventory",
			Usage: "collect inventory",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "component-type, t",
					Usage:       "Component slug to collect inventory for.",
					Destination: &collector.component,
				},
				cli.StringFlag{
					Name:        "server-url, u",
					Usage:       "server URL to submit inventory.",
					EnvVar:      "SERVER_URL",
					Required:    true,
					Destination: &collector.serverURL,
				},
				cli.StringFlag{
					Name:        "local-file, l",
					Usage:       "write inventory results to local file.",
					Required:    false,
					Destination: &collector.localFile,
				},
				cli.BoolFlag{
					Name:        "dry-run, d",
					Usage:       "collect inventory, skip posting data to server URL.",
					Required:    false,
					Destination: &collector.dryRun,
				},
				cli.BoolFlag{
					Name:        "verbose, v",
					Usage:       "Turn on verbose messages for debugging.",
					Required:    false,
					Destination: &collector.verbose,
				},
			},

			Action: func(c *cli.Context) error {
				return collector.inventory()
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

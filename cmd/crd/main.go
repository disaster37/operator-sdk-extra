package main

import (
	"fmt"
	"os"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	version = "develop"
	commit  = ""
)

func run(args []string) error {
	// Logger setting
	log.SetOutput(os.Stdout)

	// CLI settings
	app := cli.NewApp()
	app.Usage = "Clean CRD tools"
	app.Version = fmt.Sprintf("%s-%s", version, commit)
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Display debug output",
		},
		&cli.BoolFlag{
			Name:  "no-color",
			Usage: "No print color",
		},
	}
	app.Commands = []*cli.Command{
		{
			Name:  "clean-crd",
			Usage: "clean crd that contain '@clean' on description",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "crd-file",
					Usage: "The CRD files to clean. Toy can use glob path",
				},
				&cli.BoolFlag{
					Name:  "dry-run",
					Usage: "Print planned writes without modifying files",
				},
			},
			Action: CleanCrd,
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}

		if !c.Bool("no-color") {
			log.SetFormatter(&log.TextFormatter{
				FullTimestamp: true,
				ForceColors:   true, // replaces prefixed.ForceFormatting (no function-name prefix)
			})
		}

		return nil
	}

	sort.Sort(cli.CommandsByName(app.Commands))

	return app.Run(args)
}

func main() {
	err := run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

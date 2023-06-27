package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/13x-Tech/go-did-web/pkg/server"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "didsrv",
		Usage: "a did web server",
		Commands: []*cli.Command{{
			Name:  "start",
			Usage: "start service",
			Flags: []cli.Flag{&cli.StringFlag{
				Name:     "domain",
				Aliases:  []string{"d"},
				Usage:    "domain name to use for did web",
				Required: true,
			},
				&cli.StringFlag{
					Name:    "storage",
					Aliases: []string{"s"},
					Usage:   "path to directory for storage",
				}},
			Action: func(c *cli.Context) error {
				domainInput := c.String("domain")
				storageInput := c.String("storage")
				if len(storageInput) == 0 {
					homeDir, err := os.UserHomeDir()
					if err != nil {
						return err
					}
					storageInput = filepath.Join(homeDir, ".did-web", "storage")
				}

				return startServer(domainInput, storageInput)
			},
		}},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func startServer(domain, storageDir string) error {

	storage, err := server.NewStore(domain, storageDir, "did")
	if err != nil {
		return err
	}

	srv, err := server.New(
		server.WithStore(storage),
		server.WithDomain(domain),
	)
	if err != nil {
		return err
	}

	return srv.Start()
}

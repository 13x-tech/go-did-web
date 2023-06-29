package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/13x-tech/go-did-web/pkg/server"
	"github.com/13x-tech/go-did-web/pkg/storage"
	"github.com/13x-tech/go-did-web/pkg/storage/didstorage"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "didsrv",
		Usage: "a did web server",
		Commands: []*cli.Command{{
			Name:  "start",
			Usage: "start service",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "domain",
					Aliases:  []string{"d"},
					Usage:    "domain name to use for did web",
					Required: true,
				},
				&cli.StringFlag{
					Name:    "storage",
					Aliases: []string{"s"},
					Usage:   "path to directory for storage",
				},
				&cli.StringFlag{
					Name:     "apiKey",
					Aliases:  []string{"a"},
					Usage:    "lnbits api key",
					Required: true,
				},
			},
			Action: func(c *cli.Context) error {
				domainInput := c.String("domain")
				storageInput := c.String("storage")
				apiKey := c.String("apiKey")
				if len(apiKey) == 0 {
					log.Fatal(fmt.Errorf("api key is required"))
				}
				if len(storageInput) == 0 {
					homeDir, err := os.UserHomeDir()
					if err != nil {
						return err
					}
					storageInput = filepath.Join(homeDir, ".did-web", "storage")
				}

				return startServer(domainInput, storageInput, "legend.lnbits.com", apiKey)
			},
		}},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func startServer(domain, storageDir, apiHost, apiKey string) error {

	serverStore, err := server.NewStore(domain, storageDir, "did")
	if err != nil {
		return fmt.Errorf("could not load server storage: %w", err)
	}
	regStore, err := storage.New(storageDir, "reg")
	if err != nil {
		return fmt.Errorf("could not load reg storage: %w", err)
	}

	registerStore := didstorage.NewRegisterStore(apiHost, apiKey, regStore)

	srv, err := server.New(
		server.WithRegisterStore(registerStore),
		server.WithStore(serverStore),
		server.WithDomain(domain),
	)
	if err != nil {
		return err
	}

	return srv.Start()
}

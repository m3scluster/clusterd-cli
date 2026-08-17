package main

import (
	"fmt"
	"os"

	"mesos-cli/internal/app"
	"mesos-cli/internal/config"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v.\n", err)
		os.Exit(1)
	}
	application, err := app.New(cfg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v.\n", err)
		os.Exit(1)
	}
	os.Exit(application.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

package main

import (
	"os"
	"tuf-golang-project/internal/server"
)

func main() {
	cli := server.NewCLI()
	if err := cli.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
package main

import (
	"os"
	"tuf-golang-project/internal/server"
)

func main() {
	cli := server.NewCLI()
	cli.Run(os.Args)
}
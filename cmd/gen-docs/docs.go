package main

import (
	cmd "github.com/pgollangi/fireql/pkg/cmd"
	"github.com/spf13/cobra/doc"
	"log"
)

func main() {
	err := doc.GenMarkdownTree(cmd.RootCmd, "./docs")
	if err != nil {
		log.Fatal(err)
	}
	err = doc.GenReSTTree(cmd.RootCmd, "./docs")
	if err != nil {
		log.Fatal(err)
	}
}

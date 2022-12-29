package main

import (
	"github.com/pgollangi/fireql/pkg/cmd"
)

// Version is set at build
var version string

// build is set at build
var build string

func main() {
	cmd.Version = version
	cmd.Build = build
	cmd.Execute()
}

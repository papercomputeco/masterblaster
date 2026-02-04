package main

import "github.com/paper-compute-co/masterblaster/cmd"

// version is set by the linker via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cmd.Execute(version)
}

package main

import (
	"flag"
	"github.com/b1naryth1ef/bowser/lib"
)

var configPath = flag.String("config", "config.json", "path to json configuration file")

func main() {
	flag.Parse()

	sshd := bowser.NewSSHDState(*configPath)
	sshd.Run()
}

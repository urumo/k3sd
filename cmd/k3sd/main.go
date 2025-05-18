package main

import (
	"geet.svck.dev/urumo/k3sd/cluster"
	"geet.svck.dev/urumo/k3sd/utils"
)

func main() {
	utils.ParseFlags()

	if utils.Uninstall {
		cluster.UninstallCluster()
	} else {
		cluster.CreateCluster()
	}
}

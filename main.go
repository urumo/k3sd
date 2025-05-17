package main

import (
	"k3sd/cluster"
	"k3sd/utils"
)

func main() {
	utils.ParseFlags()

	if utils.Uninstall {
		cluster.UninstallCluster()
	} else {
		cluster.CreateCluster()
	}
}

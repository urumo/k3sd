package main

import (
	"geet.svck.dev/urumo/k3sd/cluster"
	"geet.svck.dev/urumo/k3sd/utils"
	"log"
)

func main() {
	utils.ParseFlags()

	if utils.Uninstall {
		cluster.UninstallCluster()
	} else {
		clusters, err := cluster.LoadClusters(utils.ConfigPath)
		if err != nil {
			log.Fatalf("failed to load clusters: %v", err)
		}
		cluster.CreateCluster(clusters)

		if err := cluster.SaveClusters(utils.ConfigPath, clusters); err != nil {
			log.Fatalf("failed to save clusters: %v", err)
		}
	}
}

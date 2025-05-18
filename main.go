package main

import (
	"geet.svck.dev/urumo/k3sd/cluster"
	"geet.svck.dev/urumo/k3sd/utils"
	"log"
)

func main() {
	go utils.LogWorker()

	utils.ParseFlags()

	clusters, err := cluster.LoadClusters(utils.ConfigPath)
	if err != nil {
		log.Fatalf("failed to load clusters: %v", err)
	}
	if utils.Uninstall {
		cluster.UninstallCluster(clusters)
	} else {
		cluster.CreateCluster(clusters)
	}
	if err := cluster.SaveClusters(utils.ConfigPath, clusters); err != nil {
		log.Fatalf("failed to save clusters: %v", err)
	}
}

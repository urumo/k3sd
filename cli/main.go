package main

import (
	"bufio"
	"fmt"
	"github.com/argon-chat/k3sd/cluster"
	"github.com/argon-chat/k3sd/utils"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	utils.ParseFlags()

	if utils.VersionFlag {
		fmt.Printf("K3SD version: %s\n", utils.Version)
		os.Exit(0)
	}

	clusters, err := cluster.LoadClusters(utils.ConfigPath)
	if err != nil {
		log.Fatalf("failed to load clusters: %v", err)
	}

	logger := utils.NewLogger("cli")
	go logger.LogWorker()
	go logger.LogWorkerErr()
	go logger.LogWorkerFile()
	go logger.LogWorkerCmd()

	checkCommandExists()

	if utils.Uninstall {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Are you sure you want to uninstall the clusters? (yes/no): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "yes" {
			clusters, err = cluster.UninstallCluster(clusters, logger)
			if err != nil {
				log.Fatalf("failed to uninstall clusters: %v", err)
			}
		} else {
			fmt.Println("Uninstallation canceled.")
			return
		}
	} else {
		clusters, err = cluster.CreateCluster(clusters, logger, []string{})
		if err != nil {
			log.Fatalf("failed to create clusters: %v", err)
		}
	}

	if err := cluster.SaveClusters(utils.ConfigPath, clusters); err != nil {
		log.Fatalf("failed to save clusters: %v", err)
	}
}

func checkCommandExists() {
	commands := []string{
		"linkerd",
		"kubectl",
		"step",
		"ssh",
	}

	for _, cmd := range commands {
		if _, err := exec.LookPath(cmd); err != nil {
			log.Fatalf("Command %s not found. Please install it.", cmd)
		}
	}
}

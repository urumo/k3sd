package main

import (
	"geet.svck.dev/urumo/k3sd/cluster"
	"geet.svck.dev/urumo/k3sd/utils"
	"log"

	"github.com/rivo/tview"
)

func main() {
	logger := utils.NewLogger("cli")
	go logger.LogWorker()
	go logger.LogWorkerErr()
	go logger.LogWorkerFile()

	utils.ParseFlags()

	clusters, err := cluster.LoadClusters(utils.ConfigPath)
	if err != nil {
		log.Fatalf("failed to load clusters: %v", err)
	}

	if utils.Uninstall {
		app := tview.NewApplication()
		modal := tview.NewModal().
			SetText("Are you sure you want to uninstall the clusters?").
			AddButtons([]string{"Yes", "No"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					cluster.UninstallCluster(clusters, logger)
					app.Stop()
				} else {
					app.Stop()
				}
			})

		if err := app.SetRoot(modal, true).Run(); err != nil {
			log.Fatalf("failed to start TUI: %v", err)
		}
	} else {
		cluster.CreateCluster(clusters, logger, []string{})
	}

	if err := cluster.SaveClusters(utils.ConfigPath, clusters); err != nil {
		log.Fatalf("failed to save clusters: %v", err)
	}
}

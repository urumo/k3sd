package cluster

import (
	"fmt"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"
)

func UninstallCluster(clusters []Cluster, logger *utils.Logger) ([]Cluster, error) {
	for ci, cluster := range clusters {
		client, err := sshConnect(cluster.User, cluster.Password, cluster.Address)
		if err != nil {
			return nil, fmt.Errorf("error connecting to cluster %s: %v", cluster.Address, err)
		}
		defer func(client *ssh.Client) {
			err := client.Close()
			if err != nil {
				logger.LogErr("Error closing SSH connection to %s: %v\n", cluster.Address, err)
			} else {
				logger.Log("SSH connection to %s closed successfully.\n", cluster.Address)
			}
		}(client)

		for wi, worker := range cluster.Workers {
			if worker.Done {
				if err := ExecuteCommands(client, []string{
					fmt.Sprintf("ssh %s@%s \"k3s-agent-uninstall.sh\"", worker.User, worker.Address),
				}, logger); err != nil {
					logger.Log("Error uninstalling worker on %s: %v\n", cluster.Address, err)
				}
				clusters[ci].Workers[wi].Done = false
			}
		}

		if cluster.Done {
			if err := ExecuteCommands(client, []string{"k3s-uninstall.sh"}, logger); err != nil {
				logger.Log("Error uninstalling master on %s: %v\n", cluster.Address, err)
			}
			clusters[ci].Done = false
		}
	}

	return clusters, nil
}

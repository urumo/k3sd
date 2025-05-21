package cluster

import (
	"fmt"
	"github.com/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
)

// UninstallCluster removes the K3s installation from the specified clusters and their workers.
//
// Parameters:
//   - clusters: A slice of Cluster objects representing the clusters to be uninstalled.
//
// Returns:
//   - []Cluster: The updated slice of Cluster objects with their statuses reset.
//   - Error: An error if any step in the uninstallation process fails.
func UninstallCluster(clusters []Cluster, logger *utils.Logger) ([]Cluster, error) {
	for ci, cluster := range clusters {
		// Configure SSH client for connecting to the cluster.
		config := &ssh.ClientConfig{
			User: cluster.User,
			Auth: []ssh.AuthMethod{
				ssh.Password(cluster.Password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		// Establish an SSH connection to the cluster.
		client, err := ssh.Dial("tcp", cluster.Address+":22", config)
		if err != nil {
			return nil, fmt.Errorf("Error connecting to cluster %s: %v\n", cluster.Address, err)
		}
		defer client.Close()

		// Uninstall K3s agent from each worker node in the cluster.
		for wi, worker := range cluster.Workers {
			if err := ExecuteCommands(client, []string{
				fmt.Sprintf("ssh %s@%s \"k3s-agent-uninstall.sh\"", worker.User, worker.Address),
			}, logger); err != nil {
				logger.Log("Error uninstalling worker on %s: %v\n", cluster.Address, err)
			}
			clusters[ci].Workers[wi].Done = false
		}

		// Uninstall K3s from the master node.
		if err := ExecuteCommands(client, []string{"k3s-uninstall.sh"}, logger); err != nil {
			logger.Log("Error uninstalling master on %s: %v\n", cluster.Address, err)
		}
		clusters[ci].Done = false
	}

	return clusters, nil
}

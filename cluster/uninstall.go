package cluster

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
)

func UninstallCluster(clusters []Cluster) ([]Cluster, error) {
	for ci, cluster := range clusters {
		config := &ssh.ClientConfig{
			User: cluster.User,
			Auth: []ssh.AuthMethod{
				ssh.Password(cluster.Password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		client, err := ssh.Dial("tcp", cluster.Address+":22", config)
		if err != nil {
			return nil, fmt.Errorf("Error connecting to cluster %s: %v\n", cluster.Address, err)
		}
		defer client.Close()

		for wi, worker := range cluster.Workers {
			if err := ExecuteCommands(client, []string{
				fmt.Sprintf("ssh %s@%s \"k3s-agent-uninstall.sh\"", worker.User, worker.Address),
			}); err != nil {
				log.Printf("Error uninstalling worker on %s: %v\n", cluster.Address, err)
			}
			clusters[ci].Workers[wi].Done = false
		}

		if err := ExecuteCommands(client, []string{"k3s-uninstall.sh"}); err != nil {
			log.Printf("Error uninstalling master on %s: %v\n", cluster.Address, err)
		}
		clusters[ci].Done = false
	}

	return clusters, nil
}

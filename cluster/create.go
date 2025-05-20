package cluster

import (
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"path"
	"strings"
)

// CreateCluster sets up a cluster and its workers by executing necessary commands remotely via SSH.
//
// Parameters:
//   - clusters: A slice of Cluster objects representing the clusters to be created.
//
// Returns:
//   - []Cluster: The updated slice of Cluster objects with their statuses updated.
//   - Error: An error if any step in the cluster creation process fails.
func CreateCluster(clusters []Cluster, logger *utils.Logger) ([]Cluster, error) {
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
			return nil, fmt.Errorf("failed to dial: %v", err)
		}
		defer client.Close()

		commands := baseClusterCommands(cluster)

		appendOptionalApps(&commands, cluster.Domain)

		if !cluster.Done {
			logger.Log("Connecting to cluster: %s\n", cluster.Address)
			if err := ExecuteCommands(client, commands, logger); err != nil {
				return nil, fmt.Errorf("Error executing commands on cluster %s: %v\n", cluster.Address, err)
			}
			clusters[ci].Done = true
		}

		for wi, worker := range cluster.Workers {
			if worker.Done {
				continue
			}
			clusters[ci].Workers[wi].Done = true

			joinToken, err := ExecuteRemoteScript(client, "echo $(k3s token create)")
			if err != nil {
				logger.Log("Error generating token on cluster %s: %v\n", cluster.Address, err)
				continue
			}

			workerCmds := []string{
				fmt.Sprintf("ssh %s@%s \"sudo apt-get update && sudo apt-get upgrade -y && sudo apt-get install curl wget -y\"", worker.User, worker.Address),
				fmt.Sprintf("ssh %s@%s \"curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' sh -\"", worker.User, worker.Address, cluster.Address, strings.TrimSpace(joinToken)),
				fmt.Sprintf("kubectl label node %s %s --overwrite", worker.NodeName, worker.Labels),
			}

			if err := ExecuteCommands(client, workerCmds, logger); err != nil {
				return nil, fmt.Errorf("Error executing worker join on cluster %s: %v\n", cluster.Address, err)
			}
		}

		saveKubeConfig(client, cluster, clusters[ci].NodeName, logger)
	}

	return clusters, nil
}

// baseClusterCommands generates a list of base commands to set up a cluster.
//
// Parameters:
//   - cluster: A Cluster object representing the cluster to be set up.
//
// Returns:
//   - []string: A slice of strings containing the commands to be executed.
func baseClusterCommands(cluster Cluster) []string {
	return []string{
		"sudo apt-get update -y",
		//"sudo apt-get upgrade -y",
		"sudo apt-get install curl wget zip unzip -y",
		//"cd /tmp && wget https://geet.svck.dev/urumo/yamls/archive/v0.0.2.zip",
		"unzip -o /tmp/v0.0.2.zip -d /tmp",
		"curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC=\"--disable traefik\" K3S_KUBECONFIG_MODE=\"644\" sh -",
		"sleep 10",
		fmt.Sprintf("kubectl label node %s %s --overwrite", cluster.NodeName, cluster.Labels),
	}
}

// appendOptionalApps appends optional application installation commands to the provided command list.
//
// Parameters:
//   - commands: A pointer to a slice of strings representing the base commands.
//   - domain: A string representing the domain name for the cluster.
func appendOptionalApps(commands *[]string, domain string) {
	if utils.Flags["prometheus"] {
		*commands = append(*commands,
			"curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash",
			"helm version",
			"helm repo add prometheus-community https://prometheus-community.github.io/helm-charts",
			"helm repo update prometheus-community",
			"KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm upgrade --install kube-prom-stack prometheus-community/kube-prometheus-stack --version \"35.5.1\" --namespace monitoring --create-namespace -f /tmp/yamls/prom-stack-values.yaml",
		)
	}
	if utils.Flags["cert-manager"] {
		*commands = append(*commands,
			"kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.crds.yaml",
			"kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml",
			"sleep 30",
		)
	}
	if utils.Flags["traefik-values"] {
		*commands = append(*commands,
			"kubectl apply -f /tmp/yamls/traefik-values.yaml",
			"while ! kubectl get deploy -n kube-system | grep -q traefik; do sleep 5; done; while [ $(kubectl get deploy -n kube-system | grep traefik | awk '{print $2}') != \"1/1\" ]; do sleep 5; done",
		)
	}
	if utils.Flags["clusterissuer"] {
		*commands = append(*commands, fmt.Sprintf("cat /tmp/yamls/clusterissuer.yaml | DOMAIN=%s envsubst | kubectl apply -f -", domain))
	}
	if utils.Flags["gitea"] {
		*commands = append(*commands, "kubectl apply -f /tmp/yamls/gitea.yaml")
		if utils.Flags["gitea-ingress"] {
			*commands = append(*commands, fmt.Sprintf("cat /tmp/yamls/gitea.ingress.yaml | DOMAIN=%s envsubst | kubectl apply -f -", domain))
		}
	}
}

// saveKubeConfig retrieves the kubeconfig file from the cluster and saves it locally.
//
// Parameters:
//   - client: An SSH client used to execute remote commands.
//   - cluster: A Cluster object representing the cluster.
//   - nodeName: A string representing the name of the node.
func saveKubeConfig(client *ssh.Client, cluster Cluster, nodeName string, logger *utils.Logger) {
	kubeConfig, err := ExecuteRemoteScript(client, "cat /etc/rancher/k3s/k3s.yaml")
	if err != nil {
		logger.Log("Failed to read kubeconfig from %s: %v\n", cluster.Address, err)
		return
	}
	kubeConfig = strings.Replace(kubeConfig, "127.0.0.1", cluster.Address, -1)

	logger.LogFile(kubeConfig)

	kubeConfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s.yaml", nodeName))
	if err := os.MkdirAll(path.Dir(kubeConfigPath), os.ModePerm); err != nil {
		log.Fatalf("Error creating directory: %v\n", err)
	}
	if err := os.WriteFile(kubeConfigPath, []byte(kubeConfig), 0644); err != nil {
		log.Fatalf("Error writing kubeconfig to file: %v\n", err)
	}
}

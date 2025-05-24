package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// getKubeClient loads a kubeconfig file and returns a Kubernetes clientset
func getKubeClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

// CreateCluster sets up a Kubernetes cluster and its workers, installs optional applications,
// and configures Linkerd if specified.
//
// Parameters:
// - clusters: A slice of Cluster objects representing the clusters to be created.
// - logger: A pointer to a utils.Logger instance for logging operations.
// - additional: A slice of additional commands to execute during cluster setup.
//
// Returns:
// - A slice of updated Cluster objects.
// - An error if any operation fails.
func CreateCluster(clusters []Cluster, logger *utils.Logger, additional []string) ([]Cluster, error) {
	for ci, cluster := range clusters {
		// Establish an SSH connection to the cluster.
		client, err := sshConnect(cluster.User, cluster.Password, cluster.Address)
		if err != nil {
			return nil, err
		}
		defer func(client *ssh.Client) {
			err := client.Close()
			if err != nil {

			}
		}(client)

		if !cluster.Done {
			// Step 1: Run only the base cluster setup commands.
			baseCmds := append(baseClusterCommands(cluster), additional...)
			logger.Log("Connecting to cluster: %s", cluster.Address)
			if err := ExecuteCommands(client, baseCmds, logger); err != nil {
				return nil, fmt.Errorf("exec master: %v", err)
			}
			cl := &clusters[ci]
			cl.Done = true

			// Step 2: Save kubeconfig to disk.
			saveKubeConfig(client, cluster, cl.NodeName, logger)

			// Step 2.5: Label the master node using client-go
			kubeconfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, cl.NodeName))
			clientset, err := getKubeClient(kubeconfigPath)
			if err != nil {
				logger.Log("Failed to create k8s client for master: %v", err)
			} else {
				labelBytes, err := json.Marshal(cluster.Labels)
				if err != nil {
					logger.Log("Failed to marshal master node labels: %v", err)
				} else {
					patch := fmt.Sprintf(`{"metadata":{"labels":%s}}`, string(labelBytes))
					_, err = clientset.CoreV1().Nodes().Patch(context.TODO(), cluster.NodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
					if err != nil {
						logger.Log("Failed to label master node %s: %v", cluster.NodeName, err)
					} else {
						logger.Log("Labeled master node %s", cluster.NodeName)
					}
				}
				if err != nil {
					logger.Log("Failed to label master node %s: %v", cluster.NodeName, err)
				} else {
					logger.Log("Labeled master node %s", cluster.NodeName)
				}
			}

			// Step 3: Apply optional apps using client-go (after kubeconfig is saved)
			if utils.Flags["cert-manager"] {
				logger.Log("Applying cert-manager CRDs and deployment...")

				err := applyYAMLManifest(kubeconfigPath, "https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml", logger, nil)
				if err != nil {
					logger.Log("cert-manager error: %v", err)
				}
				err = applyYAMLManifest(kubeconfigPath, "https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.crds.yaml", logger, nil)
				if err != nil {
					logger.Log("cert-manager CRDs error: %v", err)
				}
				logger.Log("Waiting for cert-manager deployment to be ready...")
				time.Sleep(30 * time.Second)
			}
			if utils.Flags["traefik-values"] {
				logger.Log("Applying traefik values...")
				err := applyYAMLManifest(kubeconfigPath, "yamls/traefik-values.yaml", logger, nil)
				if err != nil {
					logger.Log("traefik-values error: %v", err)
				}
				// TODO: Add wait logic for traefik deployment readiness using client-go if needed
			}
			if utils.Flags["clusterissuer"] {
				logger.Log("Applying clusterissuer...")
				substitutions := map[string]string{"${DOMAIN}": cluster.Domain, "DOMAIN": cluster.Domain}
				err := applyYAMLManifest(kubeconfigPath, "yamls/clusterissuer.yaml", logger, substitutions)
				if err != nil {
					logger.Log("clusterissuer error: %v", err)
				}
			}
			if utils.Flags["gitea"] {
				logger.Log("Applying gitea...")
				substitutions := map[string]string{
					"${POSTGRES_USER}":     cluster.Gitea.Pg.Username,
					"${POSTGRES_PASSWORD}": cluster.Gitea.Pg.Password,
					"${POSTGRES_DB}":       cluster.Gitea.Pg.DbName,
				}
				err := applyYAMLManifest(kubeconfigPath, "yamls/gitea.yaml", logger, substitutions)
				if err != nil {
					logger.Log("gitea error: %v", err)
				}
				if utils.Flags["gitea-ingress"] {
					logger.Log("Applying gitea ingress...")
					substitutions := map[string]string{"${DOMAIN}": cluster.Domain, "DOMAIN": cluster.Domain}
					err := applyYAMLManifest(kubeconfigPath, "yamls/gitea.ingress.yaml", logger, substitutions)
					if err != nil {
						logger.Log("gitea-ingress error: %v", err)
					}
				}
			}
			if utils.Flags["prometheus"] {
				logger.Log("Installing Prometheus stack via Helm Go SDK...")
				err := installHelmChart(
					kubeconfigPath,
					"kube-prom-stack",
					"monitoring",
					"prometheus-community",
					"https://prometheus-community.github.io/helm-charts",
					"kube-prometheus-stack",
					"35.5.1",
					"yamls/prom-stack-values.yaml",
					logger,
				)
				if err != nil {
					logger.Log("Prometheus stack error: %v", err)
				}
			}

			// Install Linkerd if specified in the flags.
			if utils.Flags["linkerd"] {
				runLinkerdInstall(cluster, logger, false)
			}
			if utils.Flags["linkerd-mc"] {
				runLinkerdInstall(cluster, logger, true)
			}
		}

		// Configure worker nodes for the cluster.
		for wi, worker := range cluster.Workers {
			if worker.Done {
				continue
			}
			cl := &clusters[ci].Workers[wi]
			cl.Done = true

			// Generate a token for the worker node to join the cluster.
			token, err := ExecuteRemoteScript(client, "echo $(k3s token create)", logger)
			if err != nil {
				logger.Log("token error for %s: %v", cluster.Address, err)
				continue
			}

			if cluster.PrivateNet {
				// SSH from master to worker (current logic)
				joinCmds := []string{
					fmt.Sprintf("ssh %s@%s \"sudo apt update && sudo apt install -y curl\"", worker.User, worker.Address),
					fmt.Sprintf("ssh %s@%s \"curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' INSTALL_K3S_EXEC='--node-name %s' sh -\"", worker.User, worker.Address, cluster.Address, strings.TrimSpace(token), worker.NodeName),
				}
				if err := ExecuteCommands(client, joinCmds, logger); err != nil {
					return nil, fmt.Errorf("worker join %s: %v", worker.Address, err)
				}
			} else {
				// SSH from management host to worker directly
				workerClient, err := sshConnect(worker.User, worker.Password, worker.Address)
				if err != nil {
					logger.Log("Failed to connect to worker %s directly: %v", worker.Address, err)
					continue
				}
				defer workerClient.Close()
				joinCmds := []string{
					"sudo apt update && sudo apt install -y curl",
					fmt.Sprintf("curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' INSTALL_K3S_EXEC='--node-name %s' sh -", cluster.Address, strings.TrimSpace(token), worker.NodeName),
				}
				if err := ExecuteCommands(workerClient, joinCmds, logger); err != nil {
					return nil, fmt.Errorf("worker join %s: %v", worker.Address, err)
				}
			}

			// Use client-go to label the worker node
			kubeconfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, cluster.NodeName))
			clientset, err := getKubeClient(kubeconfigPath)
			if err != nil {
				logger.Log("Failed to create k8s client: %v", err)
				continue
			}
			labelBytes, err := json.Marshal(worker.Labels)
			if err != nil {
				logger.Log("Failed to marshal worker node labels: %v", err)
			} else {
				patch := fmt.Sprintf(`{"metadata":{"labels":%s}}`, string(labelBytes))
				_, err = clientset.CoreV1().Nodes().Patch(context.TODO(), worker.NodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
				if err != nil {
					logger.Log("Failed to label node %s: %v", worker.NodeName, err)
				} else {
					logger.Log("Labeled worker node %s", worker.NodeName)
				}
			}
		}

		// Log the kubeconfig files for the cluster.
		logFiles(logger)
	}
	return clusters, nil
}

// pipeAndLog streams the output of a command to the logger.
//
// Parameters:
// - cmd: The command to execute.
// - logger: A pointer to a utils.Logger instance for logging operations.
func pipeAndLog(cmd *exec.Cmd, logger *utils.Logger) {
	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()
	_ = cmd.Start()
	go streamOutput(outPipe, false, logger)
	go streamOutput(errPipe, true, logger)
	_ = cmd.Wait()
	logger.Log("Command executed successfully")
}

// pipeAndApply streams the output of a command and applies it using kubectl.
//
// Parameters:
// - cmd: The command to execute.
// - kubeconfig: The path to the kubeconfig file.
// - Logger: A pointer to a utils.Logger instance for logging operations.
func pipeAndApply(cmd *exec.Cmd, kubeconfig string, logger *utils.Logger) {
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	_ = cmd.Start()

	var yaml strings.Builder
	go func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			yaml.WriteString(scanner.Text() + "\n")
		}
	}(stdout)
	go streamOutput(stderr, true, logger)
	_ = cmd.Wait()

	apply := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	apply.Stdin = strings.NewReader(yaml.String())
	out, err := apply.CombinedOutput()
	if err != nil {
		log.Fatalf("apply failed: %v\n%s", err, string(out))
	}
	logger.Log("Apply output:\n%s", string(out))
}

// baseClusterCommands returns a list of base commands for setting up a cluster.
//
// Parameters:
// - cluster: The Cluster object representing the cluster.
//
// Returns:
// - A slice of strings containing the base commands.
func baseClusterCommands(cluster Cluster) []string {
	return []string{
		"sudo apt-get update -y",
		"sudo apt-get install curl wget zip unzip -y",
		// No longer need to download or unzip source.zip for yamls
		fmt.Sprintf("curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--disable traefik --node-name %s' K3S_KUBECONFIG_MODE=\"644\" sh -", cluster.NodeName),
		"sleep 10",
		// Node labeling for master will be handled by client-go after kubeconfig is saved
	}
}

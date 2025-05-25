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

func logIfError(logger *utils.Logger, err error, format string, args ...interface{}) {
	if err != nil && err.Error() != "EOF" {
		logger.LogErr(format, append(args, err)...)
	}
}

func buildSubstitutions(pairs ...string) map[string]string {
	subs := make(map[string]string)
	for i := 0; i+1 < len(pairs); i += 2 {
		subs[pairs[i]] = pairs[i+1]
	}
	return subs
}

func forEachWorker(workers []Worker, fn func(*Worker) error) error {
	for i := range workers {
		if err := fn(&workers[i]); err != nil {
			return err
		}
	}
	return nil
}

func ensureNamespace(kubeconfigPath, namespace string, logger *utils.Logger) {
	if namespace != "default" && namespace != "kube-system" {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "create", "namespace", namespace)
		_ = cmd.Run()
		logger.Log("Ensured namespace %s exists", namespace)
	}
}

func installHelmRelease(component, kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) {
	logger.Log("Installing %s via Helm...", component)
	if err := installHelmChart(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile, logger); err != nil {
		logger.Log("%s Helm install error: %v", component, err)
	}
}

func applyComponentYAML(component, kubeconfigPath, manifest string, logger *utils.Logger, substitutions map[string]string) {
	logger.Log("Applying %s...", component)
	if err := applyYAMLManifest(kubeconfigPath, manifest, logger, substitutions); err != nil {
		logger.Log("%s error: %v", component, err)
	}
}

func labelNode(kubeconfigPath, nodeName string, labels map[string]string, logger *utils.Logger) error {
	clientset, err := getKubeClient(kubeconfigPath)
	if err != nil {
		logger.Log("Failed to create k8s client for node %s: %v", nodeName, err)
		return err
	}
	labelBytes, err := json.Marshal(labels)
	if err != nil {
		logger.Log("Failed to marshal node labels for %s: %v", nodeName, err)
		return err
	}
	patch := fmt.Sprintf(`{"metadata":{"labels":%s}}`, string(labelBytes))
	_, err = clientset.CoreV1().Nodes().Patch(context.TODO(), nodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		logger.Log("Failed to label node %s: %v", nodeName, err)
		return err
	} else {
		logger.Log("Labeled node %s", nodeName)
	}
	return nil
}
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

func CreateCluster(clusters []Cluster, logger *utils.Logger, additional []string) ([]Cluster, error) {
	for ci, cluster := range clusters {
		client, err := sshConnect(cluster.User, cluster.Password, cluster.Address)
		if err != nil {
			return nil, err
		}
		defer func(client *ssh.Client) {
			_ = client.Close()
		}(client)

		if !cluster.Done {
			if err := setupMasterNode(&clusters[ci], client, logger, additional); err != nil {
				return nil, err
			}
		}

		if err := setupWorkerNodes(&clusters[ci], client, logger); err != nil {
			return nil, err
		}

		logFiles(logger)
	}
	return clusters, nil
}

func setupMasterNode(cluster *Cluster, client *ssh.Client, logger *utils.Logger, additional []string) error {
	if err := runBaseClusterSetup(cluster, client, logger, additional); err != nil {
		return err
	}
	kubeconfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, cluster.NodeName))
	labelMasterNode(cluster, kubeconfigPath, logger)
	applyOptionalComponents(cluster, kubeconfigPath, logger)
	return nil
}

func runBaseClusterSetup(cluster *Cluster, client *ssh.Client, logger *utils.Logger, additional []string) error {
	baseCmds := append(baseClusterCommands(*cluster), additional...)
	logger.Log("Connecting to cluster: %s", cluster.Address)
	if err := ExecuteCommands(client, baseCmds, logger); err != nil {
		return fmt.Errorf("exec master: %v", err)
	}
	cluster.Done = true
	saveKubeConfig(client, *cluster, cluster.NodeName, logger)
	return nil
}

func labelMasterNode(cluster *Cluster, kubeconfigPath string, logger *utils.Logger) {
	_ = labelNode(kubeconfigPath, cluster.NodeName, cluster.Labels, logger)
}

func applyOptionalComponents(cluster *Cluster, kubeconfigPath string, logger *utils.Logger) {
	if utils.Flags["cert-manager"] {
		applyCertManager(kubeconfigPath, logger)
	}
	if utils.Flags["traefik-values"] {
		applyTraefikValues(kubeconfigPath, logger)
	}
	if utils.Flags["clusterissuer"] {
		applyClusterIssuer(cluster, kubeconfigPath, logger)
	}
	if utils.Flags["gitea"] {
		applyGitea(cluster, kubeconfigPath, logger)
	}
	if utils.Flags["prometheus"] {
		applyPrometheus(kubeconfigPath, logger)
	}
	if utils.Flags["linkerd"] {
		runLinkerdInstall(*cluster, logger, false)
	}
	if utils.Flags["linkerd-mc"] {
		runLinkerdInstall(*cluster, logger, true)
	}
}

func applyCertManager(kubeconfigPath string, logger *utils.Logger) {
	applyComponentYAML("cert-manager", kubeconfigPath, "https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml", logger, nil)
	applyComponentYAML("cert-manager CRDs", kubeconfigPath, "https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.crds.yaml", logger, nil)
	logger.Log("Waiting for cert-manager deployment to be ready...")
	time.Sleep(30 * time.Second)
}

func applyTraefikValues(kubeconfigPath string, logger *utils.Logger) {
	applyComponentYAML("traefik-values", kubeconfigPath, "yamls/traefik-values.yaml", logger, nil)
}

func applyClusterIssuer(cluster *Cluster, kubeconfigPath string, logger *utils.Logger) {
	substitutions := buildSubstitutions("${DOMAIN}", cluster.Domain, "DOMAIN", cluster.Domain)
	applyComponentYAML("clusterissuer", kubeconfigPath, "yamls/clusterissuer.yaml", logger, substitutions)
}

func applyGitea(cluster *Cluster, kubeconfigPath string, logger *utils.Logger) {
	substitutions := buildSubstitutions(
		"${POSTGRES_USER}", cluster.Gitea.Pg.Username,
		"${POSTGRES_PASSWORD}", cluster.Gitea.Pg.Password,
		"${POSTGRES_DB}", cluster.Gitea.Pg.DbName,
	)
	applyComponentYAML("gitea", kubeconfigPath, "yamls/gitea.yaml", logger, substitutions)
	if utils.Flags["gitea-ingress"] {
		applyGiteaIngress(cluster, kubeconfigPath, logger)
	}
}

func applyGiteaIngress(cluster *Cluster, kubeconfigPath string, logger *utils.Logger) {
	substitutions := buildSubstitutions("${DOMAIN}", cluster.Domain, "DOMAIN", cluster.Domain)
	applyComponentYAML("gitea-ingress", kubeconfigPath, "yamls/gitea.ingress.yaml", logger, substitutions)
}

func applyPrometheus(kubeconfigPath string, logger *utils.Logger) {
	ensureNamespace(kubeconfigPath, "monitoring", logger)
	installHelmRelease(
		"Prometheus stack",
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
}

func setupWorkerNodes(cluster *Cluster, client *ssh.Client, logger *utils.Logger) error {
	return forEachWorker(cluster.Workers, func(worker *Worker) error {
		if worker.Done {
			return nil
		}
		return joinAndLabelWorker(cluster, worker, client, logger)
	})
}

func joinAndLabelWorker(cluster *Cluster, worker *Worker, client *ssh.Client, logger *utils.Logger) error {
	worker.Done = true
	token, err := ExecuteRemoteScript(client, "echo $(k3s token create)", logger)
	logIfError(logger, err, "token error for %s: %v", cluster.Address)
	if err != nil {
		return nil
	}
	if err := joinWorker(cluster, worker, client, logger, token); err != nil {
		return err
	}
	return labelWorkerNode(cluster, worker, logger)
}

func joinWorker(cluster *Cluster, worker *Worker, client *ssh.Client, logger *utils.Logger, token string) error {
	if cluster.PrivateNet {
		joinCmds := []string{
			fmt.Sprintf("ssh %s@%s \"sudo apt update && sudo apt install -y curl\"", worker.User, worker.Address),
			fmt.Sprintf("ssh %s@%s \"curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' INSTALL_K3S_EXEC='--node-name %s' sh -\"", worker.User, worker.Address, cluster.Address, strings.TrimSpace(token), worker.NodeName),
		}
		if err := ExecuteCommands(client, joinCmds, logger); err != nil {
			return fmt.Errorf("worker join %s: %v", worker.Address, err)
		}
	} else {
		workerClient, err := sshConnect(worker.User, worker.Password, worker.Address)
		if err != nil {
			logger.Log("Failed to connect to worker %s directly: %v", worker.Address, err)
			return nil
		}
		defer workerClient.Close()
		joinCmds := []string{
			"sudo apt update && sudo apt install -y curl",
			fmt.Sprintf("curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' INSTALL_K3S_EXEC='--node-name %s' sh -", cluster.Address, strings.TrimSpace(token), worker.NodeName),
		}
		if err := ExecuteCommands(workerClient, joinCmds, logger); err != nil {
			return fmt.Errorf("worker join %s: %v", worker.Address, err)
		}
	}
	return nil
}

// DRY: Use generic node labeling function
func labelWorkerNode(cluster *Cluster, worker *Worker, logger *utils.Logger) error {
	kubeconfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, cluster.NodeName))
	return labelNode(kubeconfigPath, worker.NodeName, worker.Labels, logger)
}
func pipeAndLog(cmd *exec.Cmd, logger *utils.Logger) {
	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()
	_ = cmd.Start()
	go streamOutput(outPipe, false, logger)
	go streamOutput(errPipe, true, logger)
	_ = cmd.Wait()
	logger.Log("Command executed successfully")
}

func pipeAndApply(cmd *exec.Cmd, kubeconfig string, logger *utils.Logger) {
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	_ = cmd.Start()

	var yaml strings.Builder
	collectYAML(stdout, &yaml)
	go streamOutput(stderr, true, logger)
	_ = cmd.Wait()

	applyYAMLToCluster(yaml.String(), kubeconfig, logger)
}

func collectYAML(r io.Reader, yaml *strings.Builder) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		yaml.WriteString(scanner.Text() + "\n")
	}
}

func applyYAMLToCluster(yaml string, kubeconfig string, logger *utils.Logger) {
	apply := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	apply.Stdin = strings.NewReader(yaml)
	out, err := apply.CombinedOutput()
	if err != nil {
		log.Fatalf("apply failed: %v\n%s", err, string(out))
	}
	logger.Log("Apply output:\n%s", string(out))
}
func baseClusterCommands(cluster Cluster) []string {
	return []string{
		"sudo apt-get update -y",
		"sudo apt-get install curl wget zip unzip -y",
		fmt.Sprintf("curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--disable traefik --node-name %s' K3S_KUBECONFIG_MODE=\"644\" sh -", cluster.NodeName),
		"sleep 10",
	}
}

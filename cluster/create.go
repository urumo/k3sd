package cluster

import (
	"bufio"
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

func CreateCluster(clusters []Cluster, logger *utils.Logger, additionalCommands []string) ([]Cluster, error) {
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

		commands := append(baseClusterCommands(cluster), additionalCommands...)

		appendOptionalApps(&commands, cluster.Domain)

		if !cluster.Done {
			logger.Log("Connecting to cluster: %s\n", cluster.Address)
			if err := ExecuteCommands(client, commands, logger); err != nil {
				return nil, fmt.Errorf("Error executing commands on cluster %s: %v\n", cluster.Address, err)
			}
			clusters[ci].Done = true

			saveKubeConfig(client, cluster, clusters[ci].NodeName, logger)

			if utils.Flags["linkerd"] {
				installLinkerd(cluster, client, logger)
			}

			if utils.Flags["linkerd-mc"] {
				installLinkerdMC(cluster, client, logger)
			}
		}

		for wi, worker := range cluster.Workers {
			if worker.Done {
				continue
			}
			clusters[ci].Workers[wi].Done = true

			joinToken, err := ExecuteRemoteScript(client, "echo $(k3s token create)", logger)
			if err != nil {
				logger.Log("Error generating token on cluster %s: %v\n", cluster.Address, err)
				continue
			}

			workerCmds := []string{
				fmt.Sprintf("ssh %s@%s \"sudo apt-get update && sudo apt-get install curl -y\"", worker.User, worker.Address),
				fmt.Sprintf("ssh %s@%s \"curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' sh -\"", worker.User, worker.Address, cluster.Address, strings.TrimSpace(joinToken)),
				fmt.Sprintf("kubectl label node %s %s --overwrite", worker.NodeName, worker.Labels),
			}

			if err := ExecuteCommands(client, workerCmds, logger); err != nil {
				return nil, fmt.Errorf("Error executing worker join on cluster %s: %v\n", cluster.Address, err)
			}
		}

		logFiles(logger)
	}

	return clusters, nil
}

func logFiles(logger *utils.Logger) {
	dir := path.Join("./kubeconfigs", logger.Id)

	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("Error reading directory: %v\n", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := path.Join(dir, file.Name())
		content, err := os.ReadFile(filePath)

		if err != nil {
			log.Fatalf("Error reading file %s: %v\n", filePath, err)
		}

		logger.LogFile(filePath, string(content))
	}
}

func installLinkerdMC(cluster Cluster, client *ssh.Client, logger *utils.Logger) {
	installLinkerd(cluster, client, logger)

	kubeconfigPath := path.Join("./kubeconfigs", logger.Id, fmt.Sprintf("%s.yaml", cluster.NodeName))

	cmd := exec.Command("linkerd", "--kubeconfig", kubeconfigPath, "multicluster", "install")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	var yamlOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			yamlOutput.WriteString(line + "\n")
		}
	}()

	go streamOutput(stderr, true, logger)

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(yamlOutput.String())

	applyOutput, err := applyCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to apply multicluster YAML to the cluster: %v\nOutput: %s", err, string(applyOutput))
	}

	logger.Log("Linkerd multicluster installed successfully:\n%s", string(applyOutput))

	checkCmd := exec.Command("linkerd", "--kubeconfig", kubeconfigPath, "multicluster", "check")
	checkOutput, err := checkCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Linkerd multicluster check failed: %v\nOutput: %s", err, string(checkOutput))
	}

	logger.Log("Linkerd multicluster check completed successfully:\n%s", string(checkOutput))
}

func installLinkerd(cluster Cluster, client *ssh.Client, logger *utils.Logger) {
	createRootCerts(logger)
	checkLinkerdCmd(cluster, logger, true)
	installLinkerdCRDS(cluster, logger)
	installLinkerdCmd(cluster, logger)
	checkLinkerdCmd(cluster, logger, false)
}

func installLinkerdCmd(cluster Cluster, logger *utils.Logger) {
	createClusterCerts(cluster, logger)
	dir := path.Join("./kubeconfigs", logger.Id)
	kubeconfigPath := path.Join(dir, fmt.Sprintf("%s.yaml", cluster.NodeName))

	crt := fmt.Sprintf("%s/%s-issuer.crt", dir, cluster.NodeName)
	key := fmt.Sprintf("%s/%s-issuer.key", dir, cluster.NodeName)

	cmd := exec.Command("linkerd",
		"--kubeconfig", kubeconfigPath, "install",
		"--proxy-log-level=linkerd=debug,warn",
		"--cluster-domain=cluster.local",
		"--identity-trust-domain=cluster.local",
		fmt.Sprintf("--identity-trust-anchors-file=%s/ca.crt", dir),
		fmt.Sprintf("--identity-issuer-certificate-file=%s", crt),
		fmt.Sprintf("--identity-issuer-key-file=%s", key),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	var yamlOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			yamlOutput.WriteString(line + "\n")
		}
	}()

	go streamOutput(stderr, true, logger)

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(yamlOutput.String())

	applyOutput, err := applyCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to apply YAML to the cluster: %v\nOutput: %s", err, string(applyOutput))
	}

	logger.Log("Linkerd installed successfully:\n%s", string(applyOutput))
}

func createClusterCerts(cluster Cluster, logger *utils.Logger) {
	dir := path.Join("./kubeconfigs", logger.Id)

	crt := fmt.Sprintf("%s/%s-issuer.crt", dir, cluster.NodeName)
	key := fmt.Sprintf("%s/%s-issuer.key", dir, cluster.NodeName)

	cmd := exec.Command("step", "certificate", "create",
		fmt.Sprintf("identity.linkerd.%s", cluster.Domain),
		crt,
		key,
		"--ca", fmt.Sprintf("%s/ca.crt", dir),
		"--ca-key", fmt.Sprintf("%s/ca.key", dir),
		"--profile", "intermediate-ca",
		"--not-after", "438000h",
		"--no-password",
		"--insecure",
		"--force",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	logger.Log("Cluster certificates created successfully")
}

func installLinkerdCRDS(cluster Cluster, logger *utils.Logger) {
	kubeconfigPath := path.Join("./kubeconfigs", logger.Id, fmt.Sprintf("%s.yaml", cluster.NodeName))

	cmd := exec.Command("linkerd", "--kubeconfig", kubeconfigPath, "install", "--crds")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	var yamlOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			yamlOutput.WriteString(line + "\n")
		}
	}()

	go streamOutput(stderr, true, logger)

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(yamlOutput.String())

	applyOutput, err := applyCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to apply YAML to the cluster: %v\nOutput: %s", err, string(applyOutput))
	}

	logger.Log("YAML applied successfully:\n%s", string(applyOutput))
}

func checkLinkerdCmd(cluster Cluster, logger *utils.Logger, pre bool) {
	kubeconfigPath := path.Join("./kubeconfigs", logger.Id, fmt.Sprintf("%s.yaml", cluster.NodeName))
	args := []string{
		"--kubeconfig", kubeconfigPath,
		"check",
	}
	if pre {
		args = append(args, "--pre")
	}
	cmd := exec.Command("linkerd", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	logger.Log("Command executed successfully")
}

func createRootCerts(logger *utils.Logger) {
	dir := path.Join("./kubeconfigs", logger.Id)

	cmd := exec.Command("step", "certificate", "create",
		"identity.linkerd.cluster.local",
		dir+"/ca.crt",
		dir+"/ca.key",
		"--profile", "root-ca",
		"--no-password",
		"--insecure",
		"--force",
		"--not-after", "438000h",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	logger.Log("Command executed successfully")
}
func baseClusterCommands(cluster Cluster) []string {
	return []string{
		"sudo apt-get update -y",
		"sudo apt-get install curl wget zip unzip -y",
		//"cd /tmp && wget https://geet.svck.dev/urumo/yamls/archive/v0.0.2.zip",
		"unzip -o /tmp/v0.0.2.zip -d /tmp",
		"curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC=\"--disable traefik\" K3S_KUBECONFIG_MODE=\"644\" sh -",
		"sleep 10",
		fmt.Sprintf("kubectl label node %s %s --overwrite", cluster.NodeName, cluster.Labels),
	}
}

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

func saveKubeConfig(client *ssh.Client, cluster Cluster, nodeName string, logger *utils.Logger) {
	kubeConfig, err := ExecuteRemoteScript(client, "cat /etc/rancher/k3s/k3s.yaml", logger)
	if err != nil {
		logger.Log("Failed to read kubeconfig from %s: %v\n", cluster.Address, err)
		return
	}
	kubeConfig = strings.Replace(kubeConfig, "127.0.0.1", cluster.Address, -1)

	kubeConfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, nodeName))
	createFile(kubeConfigPath, kubeConfig, logger)
}

func createFile(filePath, content string, logger *utils.Logger) {
	if err := os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
		log.Fatalf("Error creating directory: %v\n", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		log.Fatalf("Error writing kubeconfig to file: %v\n", err)
	}
}

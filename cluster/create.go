package cluster

import (
	"bufio"
	"fmt"
	"github.com/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

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
			// Prepare and execute commands for setting up the cluster.
			cmds := append(baseClusterCommands(cluster), additional...)
			appendOptionalApps(&cmds, cluster.Domain, cluster.Gitea.Pg)
			logger.Log("Connecting to cluster: %s", cluster.Address)
			if err := ExecuteCommands(client, cmds, logger); err != nil {
				return nil, fmt.Errorf("exec master: %v", err)
			}
			cl := &clusters[ci]
			cl.Done = true
			saveKubeConfig(client, cluster, cl.NodeName, logger)

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

			// Commands to join the worker node to the cluster.
			joinCmds := []string{
				fmt.Sprintf("ssh %s@%s \"sudo apt update && sudo apt install -y curl\"", worker.User, worker.Address),
				fmt.Sprintf("ssh %s@%s \"curl -sfL https://get.k3s.io | K3S_URL=https://%s:6443 K3S_TOKEN='%s' sh -\"", worker.User, worker.Address, cluster.Address, strings.TrimSpace(token)),
				fmt.Sprintf("kubectl label node %s %s --overwrite", worker.NodeName, worker.Labels),
			}
			if err := ExecuteCommands(client, joinCmds, logger); err != nil {
				return nil, fmt.Errorf("worker join %s: %v", worker.Address, err)
			}
		}

		// Log the kubeconfig files for the cluster.
		logFiles(logger)
	}
	return clusters, nil
}

// sshConnect establishes an SSH connection to a remote host.
//
// Parameters:
// - user: The username for the SSH connection.
// - pass: The password for the SSH connection.
// - host: The address of the remote host.
//
// Returns:
// - A pointer to an ssh.Client instance.
// - An error if the connection fails.
func sshConnect(user, pass, host string) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return ssh.Dial("tcp", host+":22", cfg)
}

// logFiles reads and logs the contents of kubeconfig files for the cluster.
//
// Parameters:
// - logger: A pointer to a utils.Logger instance for logging operations.
func logFiles(logger *utils.Logger) {
	dir := path.Join("./kubeconfigs", logger.Id)
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("read dir: %v", err)
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fp := path.Join(dir, f.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			log.Fatalf("read file: %v", err)
		}
		logger.LogFile(fp, string(data))
	}
}

// runLinkerdInstall installs and configures Linkerd on the cluster.
//
// Parameters:
// - cluster: The Cluster object representing the cluster.
// - logger: A pointer to a utils.Logger instance for logging operations.
// - Multicluster: A boolean indicating whether to install Linkerd multicluster.
func runLinkerdInstall(cluster Cluster, logger *utils.Logger, multicluster bool) {
	dir := path.Join("./kubeconfigs", logger.Id)
	kubeconfig := path.Join(dir, fmt.Sprintf("%s.yaml", cluster.NodeName))

	createRootCerts(dir, logger)
	installCRDs(kubeconfig, logger)
	createIssuerCerts(dir, cluster, logger)
	runLinkerdCmd("install", []string{
		"--proxy-log-level=linkerd=debug,warn",
		"--cluster-domain=cluster.local",
		"--identity-trust-domain=cluster.local",
		"--identity-trust-anchors-file=" + path.Join(dir, "ca.crt"),
		"--identity-issuer-certificate-file=" + path.Join(dir, fmt.Sprintf("%s-issuer.crt", cluster.NodeName)),
		"--identity-issuer-key-file=" + path.Join(dir, fmt.Sprintf("%s-issuer.key", cluster.NodeName)),
		"--kubeconfig", kubeconfig,
	}, logger, kubeconfig, true)

	if multicluster {
		runLinkerdCmd("multicluster", []string{"install", "--kubeconfig", kubeconfig}, logger, kubeconfig, true)
		logger.Log("Linkerd multicluster installed.")
		runLinkerdCmd("multicluster", []string{"check", "--kubeconfig", kubeconfig}, logger, kubeconfig, false)
	} else {
		runLinkerdCmd("check", []string{"--pre", "--kubeconfig", kubeconfig}, logger, kubeconfig, true)
		runLinkerdCmd("check", []string{"--kubeconfig", kubeconfig}, logger, kubeconfig, false)
	}
}

// runLinkerdCmd executes a Linkerd command with the specified arguments.
//
// Parameters:
// - cmd: The Linkerd command to execute.
// - args: A slice of arguments for the command.
// - logger: A pointer to a utils.Logger instance for logging operations.
// - kubeconfig: The path to the kubeconfig file.
// - Apply: A boolean indicating whether to apply the command output.
func runLinkerdCmd(cmd string, args []string, logger *utils.Logger, kubeconfig string, apply bool) {
	parts := append([]string{cmd}, args...)
	c := exec.Command("linkerd", parts...)
	if apply {
		pipeAndApply(c, kubeconfig, logger)
	} else {
		pipeAndLog(c, logger)
	}
}

// installCRDs installs the Linkerd CRDs on the cluster.
//
// Parameters:
// - kubeconfig: The path to the kubeconfig file.
// - logger: A pointer to a utils.Logger instance for logging operations.
func installCRDs(kubeconfig string, logger *utils.Logger) {
	run := exec.Command("linkerd", "install", "--crds", "--kubeconfig", kubeconfig)
	pipeAndApply(run, kubeconfig, logger)
}

// createRootCerts generates root certificates for Linkerd.
//
// Parameters:
// - dir: The directory to store the certificates.
// - cluster: The Cluster object representing the cluster.
// - Logger: A pointer to a utils.Logger instance for logging operations.
func createRootCerts(dir string, logger *utils.Logger) {
	cmd := exec.Command("step", "certificate", "create",
		"identity.linkerd.cluster.local",
		path.Join(dir, "ca.crt"),
		path.Join(dir, "ca.key"),
		"--profile", "root-ca",
		"--no-password", "--insecure", "--force", "--not-after", "438000h",
	)
	pipeAndLog(cmd, logger)
}

// createIssuerCerts generates issuer certificates for Linkerd.
//
// Parameters:
// - dir: The directory to store the certificates.
// - cluster: The Cluster object representing the cluster.
// - Logger: A pointer to a utils.Logger instance for logging operations.
func createIssuerCerts(dir string, cluster Cluster, logger *utils.Logger) {
	cmd := exec.Command("step", "certificate", "create",
		fmt.Sprintf("identity.linkerd.%s", cluster.Domain),
		path.Join(dir, fmt.Sprintf("%s-issuer.crt", cluster.NodeName)),
		path.Join(dir, fmt.Sprintf("%s-issuer.key", cluster.NodeName)),
		"--ca", path.Join(dir, "ca.crt"),
		"--ca-key", path.Join(dir, "ca.key"),
		"--profile", "intermediate-ca",
		"--not-after", "438000h",
		"--no-password", "--insecure", "--force",
	)
	pipeAndLog(cmd, logger)
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
		fmt.Sprintf("cd /tmp && curl -L -o source.zip $(curl -s https://api.github.com/repos/urumo/k3sd/releases/tags/%s | grep \"zipball_url\" | cut -d '\"' -f 4)", utils.Version),
		"unzip -o -j /tmp/source.zip -d /tmp/yamls",
		"curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC=\"--disable traefik\" K3S_KUBECONFIG_MODE=\"644\" sh -",
		"sleep 10",
		fmt.Sprintf("kubectl label node %s %s --overwrite", cluster.NodeName, cluster.Labels),
	}
}

// appendOptionalApps appends optional application installation commands to the provided command list.
//
// Parameters:
// - commands: A pointer to a slice of strings containing the commands.
// - domain: The domain name for the cluster.
func appendOptionalApps(commands *[]string, domain string, pg Pg) {
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
		*commands = append(*commands, fmt.Sprintf("cat /tmp/yamls/gitea.yaml | POSTGRES_USER=%s POSTGRES_PASSWORD=%s POSTGRES_DB=%s  envsubst | kubectl apply -f -", pg.Username, pg.Password, pg.DbName))
		if utils.Flags["gitea-ingress"] {
			*commands = append(*commands, fmt.Sprintf("cat /tmp/yamls/gitea.ingress.yaml | DOMAIN=%s envsubst | kubectl apply -f -", domain))
		}
	}
}

// saveKubeConfig retrieves and saves the kubeconfig file for the cluster.
//
// Parameters:
// - client: A pointer to an ssh.Client instance for the SSH connection.
// - cluster: The Cluster object representing the cluster.
// - nodeName: The name of the node.
// - logger: A pointer to a utils.Logger instance for logging operations.
func saveKubeConfig(client *ssh.Client, cluster Cluster, nodeName string, logger *utils.Logger) {
	kubeConfig, err := ExecuteRemoteScript(client, "cat /etc/rancher/k3s/k3s.yaml", logger)
	if err != nil {
		logger.Log("Failed to read kubeconfig from %s: %v\n", cluster.Address, err)
		return
	}
	kubeConfig = strings.Replace(kubeConfig, "127.0.0.1", cluster.Address, -1)

	kubeConfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, nodeName))
	createFile(kubeConfigPath, kubeConfig)
}

// createFile creates a file with the specified content.
//
// Parameters:
// - filePath: The path to the file to be created.
// - content: The content to write to the file.
// - Logger: A pointer to a utils.Logger instance for logging operations.
func createFile(filePath, content string) {
	if err := os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
		log.Fatalf("Error creating directory: %v\n", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		log.Fatalf("Error writing kubeconfig to file: %v\n", err)
	}
}

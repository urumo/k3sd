package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached/memory"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// installHelmChart installs or upgrades a Helm chart using the Go SDK, given repo, chart, version, and optional values file.
func installHelmChart(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) error {
	// Ensure namespace exists before installing the chart
	if namespace != "" && namespace != "default" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to build kubeconfig for namespace creation: %w", err)
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create k8s client for namespace creation: %w", err)
		}
		_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
				}
				logger.Log("Created namespace %s", namespace)
			} else {
				return fmt.Errorf("failed to check namespace %s: %w", namespace, err)
			}
		}
	}
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), logger.Log); err != nil {
		return fmt.Errorf("failed to init helm action config: %w", err)
	}

	install := action.NewUpgrade(actionConfig)
	install.Namespace = namespace
	install.Install = true
	// install.CreateNamespace = true // Not available on Upgrade action
	install.Atomic = true
	install.Wait = true
	install.Timeout = 300 // seconds

	// Add repo if needed
	repoEntry := &repo.Entry{
		Name: repoName,
		URL:  repoURL,
	}
	// Use only helm.sh/helm/v3/pkg/getter and pass settings as environment.EnvSettings
	// getter.All expects environment.EnvSettings, which is an interface implemented by *cli.EnvSettings
	providers := getter.All(settings)
	r, err := repo.NewChartRepository(repoEntry, providers)
	if err != nil {
		return fmt.Errorf("failed to create chart repo: %w", err)
	}
	_, err = r.DownloadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to download repo index: %w", err)
	}

	// Load values
	vals := map[string]interface{}{}
	if valuesFile != "" {
		valuesBytes, err := ioutil.ReadFile(valuesFile)
		if err != nil {
			return fmt.Errorf("failed to read values file: %w", err)
		}
		if err := yaml.Unmarshal(valuesBytes, &vals); err != nil {
			return fmt.Errorf("failed to unmarshal values: %w", err)
		}
	}

	// Pull chart
	fullChart := fmt.Sprintf("%s/%s", repoName, chartName)
	install.ChartPathOptions.Version = chartVersion
	cp, err := install.ChartPathOptions.LocateChart(fullChart, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}
	chart, err := loader.Load(cp)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Run upgrade/install
	_, err = install.Run(releaseName, chart, vals)
	if err != nil {
		return fmt.Errorf("failed to install/upgrade chart: %w", err)
	}
	logger.Log("Helm chart %s/%s installed/upgraded successfully.", repoName, chartName)
	return nil
}

// applyYAMLManifest applies a YAML manifest (from URL or file) using client-go dynamic client
func applyYAMLManifest(kubeconfigPath, manifestPathOrURL string, logger *utils.Logger, substitutions map[string]string) error {
	var data []byte
	var err error
	if strings.HasPrefix(manifestPathOrURL, "http://") || strings.HasPrefix(manifestPathOrURL, "https://") {
		resp, err := http.Get(manifestPathOrURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	} else {
		data, err = os.ReadFile(manifestPathOrURL)
		if err != nil {
			return err
		}
	}
	// Substitute variables if needed
	if substitutions != nil {
		content := string(data)
		for k, v := range substitutions {
			content = strings.ReplaceAll(content, k, v)
		}
		data = []byte(content)
	}
	// Split multi-doc YAML, robustly handle document boundaries
	var docs []string
	rawDocs := strings.Split(string(data), "\n---")
	for _, doc := range rawDocs {
		doc = strings.TrimSpace(doc)
		if doc == "" || strings.HasPrefix(doc, "#") {
			continue
		}
		docs = append(docs, doc)
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	decUnstructured := yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))
	for _, doc := range docs {
		obj := &unstructured.Unstructured{}
		_, _, err := decUnstructured.Decode([]byte(doc), nil, obj)
		if err != nil {
			logger.Log("YAML decode error: %v\n---\n%s", err, doc)
			continue
		}
		m := obj.GroupVersionKind()
		mapping, err := mapper.RESTMapping(m.GroupKind(), m.Version)
		if err != nil {
			logger.Log("RESTMapping error: %v", err)
			continue
		}
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		resource := dyn.Resource(mapping.Resource).Namespace(ns)
		_, err = resource.Create(context.TODO(), obj, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			logger.Log("Apply error: %v", err)
		}
	}
	return nil
}

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
		// No longer need to download or unzip source.zip for yamls
		fmt.Sprintf("curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='--disable traefik --node-name %s' K3S_KUBECONFIG_MODE=\"644\" sh -", cluster.NodeName),
		"sleep 10",
		// Node labeling for master will be handled by client-go after kubeconfig is saved
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

	// Use client-go to parse and update kubeconfig
	config, err := clientcmd.Load([]byte(kubeConfig))
	if err != nil {
		logger.Log("Failed to parse kubeconfig: %v", err)
		return
	}
	// Rename cluster, context, and user keys to nodeName
	// (Assume only one entry for each in the original config)
	var oldCluster, oldContext, oldUser string
	for k := range config.Clusters {
		oldCluster = k
		break
	}
	for k := range config.Contexts {
		oldContext = k
		break
	}
	for k := range config.AuthInfos {
		oldUser = k
		break
	}
	if oldCluster != "" && oldCluster != nodeName {
		config.Clusters[nodeName] = config.Clusters[oldCluster]
		delete(config.Clusters, oldCluster)
	}
	if oldUser != "" && oldUser != nodeName {
		config.AuthInfos[nodeName] = config.AuthInfos[oldUser]
		delete(config.AuthInfos, oldUser)
	}
	if oldContext != "" && oldContext != nodeName {
		config.Contexts[nodeName] = config.Contexts[oldContext]
		delete(config.Contexts, oldContext)
	}
	// Update context to point to new cluster/user names
	if ctx, ok := config.Contexts[nodeName]; ok {
		ctx.Cluster = nodeName
		ctx.AuthInfo = nodeName
	}
	config.CurrentContext = nodeName
	newKubeConfig, err := clientcmd.Write(*config)
	if err != nil {
		logger.Log("Failed to marshal kubeconfig: %v", err)
		return
	}
	kubeConfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", logger.Id, nodeName))
	createFile(kubeConfigPath, string(newKubeConfig))
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

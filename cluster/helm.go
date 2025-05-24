package cluster

import (
	"fmt"
	"os/exec"

	"github.com/argon-chat/k3sd/utils"
)

func installHelmChart(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) error {
	// 1. Add the Helm repo
	logger.Log("Adding Helm repo: %s -> %s", repoName, repoURL)
	addRepoCmd := exec.Command("helm", "repo", "add", repoName, repoURL)
	pipeAndLog(addRepoCmd, logger)

	// 2. Update the Helm repos
	logger.Log("Updating Helm repos")
	updateRepoCmd := exec.Command("helm", "repo", "update")
	pipeAndLog(updateRepoCmd, logger)

	// 3. Install/Upgrade the chart
	logger.Log("Installing Helm chart: %s/%s version %s in namespace %s", repoName, chartName, chartVersion, namespace)
	installArgs := []string{
		"upgrade", "--install", releaseName,
		fmt.Sprintf("%s/%s", repoName, chartName),
		"--namespace", namespace,
		"--version", chartVersion,
		"--values", valuesFile,
		"--kubeconfig", kubeconfigPath,
		"--create-namespace",
	}
	installCmd := exec.Command("helm", installArgs...)
	pipeAndLog(installCmd, logger)

	return nil
}

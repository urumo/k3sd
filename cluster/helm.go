package cluster

import (
	"github.com/argon-chat/k3sd/utils"
)

// installHelmChart now uses the native Helm Go client (see helm_native.go)
func installHelmChart(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) error {
	return installHelmChartNative(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile, logger)
}

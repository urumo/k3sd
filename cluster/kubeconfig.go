package cluster

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// DRY: Extract old key from map
func getFirstKey(m map[string]interface{}) string {
	for k := range m {
		return k
	}
	return ""
}

func patchKubeConfigKeys(config *clientcmdapi.Config, nodeName string) {
	patchKey := func(m map[string]interface{}, set func(string), del func(string)) {
		oldKey := getFirstKey(m)
		if oldKey != "" && oldKey != nodeName {
			set(oldKey)
			del(oldKey)
		}
	}
	patchKey(toMapInterface(config.Clusters),
		func(old string) { config.Clusters[nodeName] = config.Clusters[old] },
		func(old string) { delete(config.Clusters, old) },
	)
	patchKey(toMapInterface(config.AuthInfos),
		func(old string) { config.AuthInfos[nodeName] = config.AuthInfos[old] },
		func(old string) { delete(config.AuthInfos, old) },
	)
	patchKey(toMapInterface(config.Contexts),
		func(old string) { config.Contexts[nodeName] = config.Contexts[old] },
		func(old string) { delete(config.Contexts, old) },
	)
	if ctx, ok := config.Contexts[nodeName]; ok {
		ctx.Cluster = nodeName
		ctx.AuthInfo = nodeName
	}
	config.CurrentContext = nodeName
}

func toMapInterface[T any](m map[string]T) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
func saveKubeConfig(client *ssh.Client, cluster Cluster, nodeName string, logger *utils.Logger) {
	kubeConfig, err := readRemoteKubeConfig(client, cluster.Address, logger)
	if err != nil {
		return
	}
	config, err := parseAndPatchKubeConfig(kubeConfig, cluster.Address, nodeName, logger)
	if err != nil {
		return
	}
	writeKubeConfigToFile(config, logger.Id, nodeName, logger)
}

func readRemoteKubeConfig(client *ssh.Client, address string, logger *utils.Logger) (string, error) {
	kubeConfig, err := ExecuteRemoteScript(client, "cat /etc/rancher/k3s/k3s.yaml", logger)
	if err != nil {
		logger.Log("Failed to read kubeconfig from %s: %v\n", address, err)
		return "", err
	}
	return kubeConfig, nil
}

func parseAndPatchKubeConfig(kubeConfig, address, nodeName string, logger *utils.Logger) (*clientcmdapi.Config, error) {
	kubeConfig = strings.Replace(kubeConfig, "127.0.0.1", address, -1)
	config, err := clientcmd.Load([]byte(kubeConfig))
	if err != nil {
		logger.Log("Failed to parse kubeconfig: %v", err)
		return nil, err
	}
	patchKubeConfigKeys(config, nodeName)
	return config, nil
}

func writeKubeConfigToFile(config *clientcmdapi.Config, loggerId, nodeName string, logger *utils.Logger) {
	newKubeConfig, err := clientcmd.Write(*config)
	if err != nil {
		logger.Log("Failed to marshal kubeconfig: %v", err)
		return
	}
	kubeConfigPath := path.Join("./kubeconfigs", fmt.Sprintf("%s/%s.yaml", loggerId, nodeName))
	if err := createFileWithErr(kubeConfigPath, string(newKubeConfig)); err != nil {
		logger.Log("Failed to write kubeconfig to file: %v", err)
	}
}

func createFileWithErr(filePath, content string) error {
	if err := os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("error writing kubeconfig to file: %v", err)
	}
	return nil
}

func logFiles(logger *utils.Logger) {
	dir := path.Join("./kubeconfigs", logger.Id)
	files, err := os.ReadDir(dir)
	if err != nil {
		logger.Log("read dir: %v", err)
		return
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fp := path.Join(dir, f.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			logger.Log("read file: %v", err)
			continue
		}
		logger.LogFile(fp, string(data))
	}
}

package cluster

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/tools/clientcmd"
)

func saveKubeConfig(client *ssh.Client, cluster Cluster, nodeName string, logger *utils.Logger) {
	kubeConfig, err := ExecuteRemoteScript(client, "cat /etc/rancher/k3s/k3s.yaml", logger)
	if err != nil {
		logger.Log("Failed to read kubeconfig from %s: %v\n", cluster.Address, err)
		return
	}
	kubeConfig = strings.Replace(kubeConfig, "127.0.0.1", cluster.Address, -1)

	config, err := clientcmd.Load([]byte(kubeConfig))
	if err != nil {
		logger.Log("Failed to parse kubeconfig: %v", err)
		return
	}
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

func createFile(filePath, content string) {
	if err := os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
		log.Fatalf("Error creating directory: %v\n", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		log.Fatalf("Error writing kubeconfig to file: %v\n", err)
	}
}

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

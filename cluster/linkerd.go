package cluster

import (
	"fmt"
	"os/exec"
	"path"

	"github.com/argon-chat/k3sd/utils"
)

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

func runLinkerdCmd(cmd string, args []string, logger *utils.Logger, kubeconfig string, apply bool) {
	parts := append([]string{cmd}, args...)
	c := exec.Command("linkerd", parts...)
	if apply {
		pipeAndApply(c, kubeconfig, logger)
	} else {
		pipeAndLog(c, logger)
	}
}

func installCRDs(kubeconfig string, logger *utils.Logger) {
	run := exec.Command("linkerd", "install", "--crds", "--kubeconfig", kubeconfig)
	pipeAndApply(run, kubeconfig, logger)
}

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

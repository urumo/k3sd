package utils

import (
	"flag"
	"fmt"
)

var (
	Flags       map[string]bool
	ConfigPath  string
	Uninstall   bool
	VersionFlag bool
)

func ParseFlags() {
	certManager := flag.Bool("cert-manager", false, "Apply the cert-manager YAMLs")
	traefik := flag.Bool("traefik", false, "Apply the Traefik YAML")
	clusterIssuer := flag.Bool("cluster-issuer", false, "Apply the Cluster Issuer YAML, need to specify `domain` in your config json")
	gitea := flag.Bool("gitea", false, "Apply the Gitea YAML")
	giteaIngress := flag.Bool("gitea-ingress", false, "Apply the Gitea Ingress YAML, need to specify `domain` in your config json")
	configPath := flag.String("config-path", "", "Path to clusters.json")
	prometheus := flag.Bool("prometheus", false, "Apply the Prometheus YAML")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall the cluster")
	linkerd := flag.Bool("linkerd", false, "Install linkerd")
	linkerdMc := flag.Bool("linkerd-mc", false, "Install linkerd multicluster(will install linkerd first)")
	versionFlag := flag.Bool("version", false, "Print the version and exit")

	flag.Parse()

	VersionFlag = *versionFlag
	Uninstall = *uninstallFlag
	Flags = map[string]bool{
		"cert-manager":   *certManager,
		"traefik-values": *traefik,
		"clusterissuer":  *clusterIssuer,
		"gitea":          *gitea,
		"prometheus":     *prometheus,
		"gitea-ingress":  *giteaIngress,
		"linkerd":        *linkerd,
		"linkerd-mc":     *linkerdMc,
	}

	if *configPath != "" {
		ConfigPath = *configPath
	} else if !VersionFlag {
		fmt.Println("Must specify --config-path")
		flag.Usage()
	}
}

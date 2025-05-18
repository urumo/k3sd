package utils

import (
	"flag"
	"log"
)

var (
	Flags      map[string]bool
	ConfigPath string
	Uninstall  bool
)

func ParseFlags() {
	certManager := flag.Bool("cert-manager", false, "Apply the cert-manager YAMLs")
	traefik := flag.Bool("traefik", false, "Apply the Traefik YAML")
	clusterIssuer := flag.Bool("cluster-issuer", false, "Apply the Cluster Issuer YAML")
	gitea := flag.Bool("gitea", false, "Apply the Gitea YAML")
	giteaIngress := flag.Bool("gitea-ingress", false, "Apply the Gitea Ingress YAML")
	configPath := flag.String("config-path", "", "Path to clusters.json")
	prometheus := flag.Bool("prometheus", false, "Apply the Prometheus YAML")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall the cluster")

	flag.Parse()

	Uninstall = *uninstallFlag
	Flags = map[string]bool{
		"cert-manager":   *certManager,
		"traefik-values": *traefik,
		"clusterissuer":  *clusterIssuer,
		"gitea":          *gitea,
		"prometheus":     *prometheus,
		"gitea-ingress":  *giteaIngress,
	}

	if *configPath != "" {
		ConfigPath = *configPath
	} else {
		log.Fatalf("Must specify --config-path")
	}
}

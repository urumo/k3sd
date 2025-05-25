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
	Verbose     bool
	HelmAtomic  bool
)

type boolFlagDef struct {
	Name        string
	Default     bool
	Description string
	MapKey      string
}

func ParseFlags() {
	boolFlags := []boolFlagDef{
		{"cert-manager", false, "Apply the cert-manager YAMLs", "cert-manager"},
		{"traefik", false, "Apply the Traefik YAML", "traefik-values"},
		{"cluster-issuer", false, "Apply the Cluster Issuer YAML, need to specify `domain` in your config json", "clusterissuer"},
		{"gitea", false, "Apply the Gitea YAML", "gitea"},
		{"gitea-ingress", false, "Apply the Gitea Ingress YAML, need to specify `domain` in your config json", "gitea-ingress"},
		{"prometheus", false, "Apply the Prometheus YAML", "prometheus"},
		{"linkerd", false, "Install linkerd", "linkerd"},
		{"linkerd-mc", false, "Install linkerd multicluster(will install linkerd first)", "linkerd-mc"},
	}

	flagPtrs := make(map[string]*bool)
	for _, def := range boolFlags {
		flagPtrs[def.MapKey] = flag.Bool(def.Name, def.Default, def.Description)
	}

	configPath := flag.String("config-path", "", "Path to clusters.json")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall the cluster")
	versionFlag := flag.Bool("version", false, "Print the version and exit")
	verbose := flag.Bool("v", false, "Enable verbose stdout logging")
	helmAtomic := flag.Bool("helm-atomic", false, "Enable --atomic for all Helm operations (rollback on failure)")

	flag.Parse()

	VersionFlag = *versionFlag
	Uninstall = *uninstallFlag
	Verbose = *verbose
	HelmAtomic = *helmAtomic

	Flags = make(map[string]bool)
	for k, ptr := range flagPtrs {
		Flags[k] = *ptr
	}

	if *configPath != "" {
		ConfigPath = *configPath
	} else if !VersionFlag {
		fmt.Println("Must specify --config-path")
		flag.Usage()
	}
}

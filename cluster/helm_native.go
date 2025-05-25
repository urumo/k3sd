package cluster

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/argon-chat/k3sd/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

func installHelmChartNative(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) error {
	settings := cli.New()
	settings.KubeConfig = kubeconfigPath
	settings.SetNamespace(namespace)
	settings.Debug = utils.Verbose

	repoFile := settings.RepositoryConfig
	repoEntry := &repo.Entry{
		Name: repoName,
		URL:  repoURL,
	}
	rf, err := repo.LoadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load repo file: %w", err)
	}
	if rf.Has(repoName) {
		logger.Log("Helm repo %s already exists", repoName)
	} else {
		r, err := repo.NewChartRepository(repoEntry, getter.All(settings))
		if err != nil {
			return fmt.Errorf("failed to create chart repo: %w", err)
		}
		if _, err := r.DownloadIndexFile(); err != nil {
			return fmt.Errorf("failed to download repo index: %w", err)
		}
		rf.Update(repoEntry)
		if err := rf.WriteFile(repoFile, 0644); err != nil {
			return fmt.Errorf("failed to write repo file: %w", err)
		}
		logger.Log("Added Helm repo: %s -> %s", repoName, repoURL)
	}

	rf2, err := repo.LoadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load repo file for update: %w", err)
	}
	for _, re := range rf2.Repositories {
		r, err := repo.NewChartRepository(re, getter.All(settings))
		if err != nil {
			return fmt.Errorf("failed to create chart repo for update: %w", err)
		}
		if _, err := r.DownloadIndexFile(); err != nil {
			return fmt.Errorf("failed to update repo %s: %w", re.Name, err)
		}
	}
	logger.Log("Helm repos updated")

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), logger.Log); err != nil {
		return fmt.Errorf("failed to init helm action config: %w", err)
	}

	if namespace != "default" && namespace != "kube-system" {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "create", "namespace", namespace)
		_ = cmd.Run() // ignore error if already exists
	}

	valOpts := &values.Options{ValueFiles: []string{valuesFile}}
	valMap, err := valOpts.MergeValues(getter.All(settings))
	if err != nil {
		return fmt.Errorf("failed to parse values file: %w", err)
	}
	chartPathOpts := &action.ChartPathOptions{Version: chartVersion}
	chartRef := fmt.Sprintf("%s/%s", repoName, chartName)
	chartPath, err := chartPathOpts.LocateChart(chartRef, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	rels := action.NewList(actionConfig)
	rels.All = true
	rels.SetStateMask()
	found := false
	releases, err := rels.Run()
	if err == nil {
		for _, r := range releases {
			if r.Name == releaseName && r.Namespace == namespace {
				found = true
				break
			}
		}
	}

	if !found {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = releaseName
		install.Namespace = namespace
		install.Version = chartVersion
		install.Atomic = utils.HelmAtomic
		install.Wait = true
		install.Timeout = 600 * time.Second
		install.CreateNamespace = true

		rel, err := install.Run(ch, valMap)

		if err != nil {
			logger.LogErr("Helm install error: %v", err)
			if rel != nil && rel.Info != nil {
				logger.LogErr("Helm release info: %s", rel.Info.Description)
			}
			return fmt.Errorf("helm install failed: %w", err)
		}
		if rel != nil && rel.Info != nil {
			logger.Log("Helm release info: %s", rel.Info.Description)
		}
		logger.Log("Helm chart %s installed successfully", chartRef)
		return nil
	} else {
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = namespace
		upgrade.Install = false
		upgrade.Version = chartVersion
		upgrade.Atomic = utils.HelmAtomic
		upgrade.Wait = true
		upgrade.Timeout = 600

		rel, err := upgrade.Run(releaseName, ch, valMap)

		if err != nil {
			logger.LogErr("Helm upgrade error: %v", err)
			if rel != nil && rel.Info != nil {
				logger.LogErr("Helm release info: %s", rel.Info.Description)
			}
			return fmt.Errorf("helm upgrade failed: %w", err)
		}
		if rel != nil && rel.Info != nil {
			logger.Log("Helm release info: %s", rel.Info.Description)
		}
		logger.Log("Helm chart %s upgraded successfully", chartRef)
		return nil
	}
}

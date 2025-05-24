package cluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/argon-chat/k3sd/utils"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// installHelmChart installs or upgrades a Helm chart using the Go SDK, given repo, chart, version, and optional values file.
func installHelmChart(kubeconfigPath, releaseName, namespace, repoName, repoURL, chartName, chartVersion, valuesFile string, logger *utils.Logger) error {
	settings := cli.New()
	helmDataDir := "./.helm"
	err := os.MkdirAll(helmDataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create local helm data dir: %w", err)
	}
	settings.RepositoryConfig = path.Join(helmDataDir, "repositories.yaml")
	settings.RepositoryCache = path.Join(helmDataDir, "cache")

	// Always update all repo caches before installing a chart (mimics 'helm repo update')
	repoFile := settings.RepositoryConfig
	reposForUpdate, err := repo.LoadFile(repoFile)
	if err == nil && reposForUpdate != nil {
		providers := getter.All(settings)
		if err := os.MkdirAll(settings.RepositoryCache, 0755); err != nil {
			return fmt.Errorf("failed to create helm repo cache dir: %w", err)
		}
		for _, rEntry := range reposForUpdate.Repositories {
			rc, err := repo.NewChartRepository(rEntry, providers)
			if err != nil {
				logger.Log("Failed to create chart repo for update: %v", err)
				continue
			}
			if _, err := rc.DownloadIndexFile(); err != nil {
				logger.Log("Failed to update repo index for %s: %v", rEntry.Name, err)
			}
		}
	}

	// Ensure namespace exists before installing the chart (handled in create.go)
	actionConfig := new(action.Configuration)
	err = actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), logger.Log)
	if err != nil {
		return fmt.Errorf("failed to init helm action config: %w", err)
	}

	install := action.NewUpgrade(actionConfig)
	install.Namespace = namespace
	install.Install = true
	install.Atomic = true
	install.Wait = true
	install.Timeout = 300 // seconds

	// Add repo if needed (idempotent)
	repoFile = settings.RepositoryConfig
	err = os.MkdirAll(path.Dir(repoFile), 0755)
	if err != nil {
		return fmt.Errorf("failed to create helm repo config dir: %w", err)
	}
	if _, statErr := os.Stat(repoFile); os.IsNotExist(statErr) {
		emptyRepo := []byte("apiVersion: v1\ngenerated: \"1970-01-01T00:00:00.000Z\"\nrepositories: []\n")
		err = os.WriteFile(repoFile, emptyRepo, 0644)
		if err != nil {
			return fmt.Errorf("failed to create empty helm repo file: %w", err)
		}
	}
	repos, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load helm repo file: %w", err)
	}
	found := false
	if repos != nil {
		for _, r := range repos.Repositories {
			if r.Name == repoName {
				found = true
				break
			}
		}
	}
	if !found {
		repoEntry := &repo.Entry{
			Name: repoName,
			URL:  repoURL,
		}
		providers := getter.All(settings)
		r, err := repo.NewChartRepository(repoEntry, providers)
		if err != nil {
			return fmt.Errorf("failed to create chart repo: %w", err)
		}
		if err := os.MkdirAll(settings.RepositoryCache, 0755); err != nil {
			return fmt.Errorf("failed to create helm repo cache dir: %w", err)
		}
		if _, err = r.DownloadIndexFile(); err != nil {
			return fmt.Errorf("failed to download repo index: %w", err)
		}
		cacheFile := path.Join(settings.RepositoryCache, repoName+"-index.yaml")
		if _, statErr := os.Stat(cacheFile); os.IsNotExist(statErr) {
			logger.Log("Helm repo index cache file missing after DownloadIndexFile: %s", cacheFile)
			return fmt.Errorf("repo index cache file not created: %s", cacheFile)
		}
		if repos == nil {
			repos = repo.NewFile()
		}
		repos.Update(repoEntry)
		if err := repos.WriteFile(repoFile, 0644); err != nil {
			return fmt.Errorf("failed to write helm repo file: %w", err)
		}
		logger.Log("Added helm repo %s", repoName)
	}

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

	_, err = install.Run(releaseName, chart, vals)
	if err != nil {
		return fmt.Errorf("failed to install/upgrade chart: %w", err)
	}
	logger.Log("Helm chart %s/%s installed/upgraded successfully.", repoName, chartName)
	return nil
}

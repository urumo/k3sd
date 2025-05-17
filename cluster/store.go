package cluster

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadClusters(path string) ([]Cluster, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open cluster config: %w", err)
	}
	defer file.Close()

	var clusters []Cluster
	if err := json.NewDecoder(file).Decode(&clusters); err != nil {
		return nil, fmt.Errorf("decode cluster config: %w", err)
	}
	return clusters, nil
}

func SaveClusters(path string, clusters []Cluster) error {
	data, err := json.MarshalIndent(clusters, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cluster config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

package cluster

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadClusters reads a JSON file from the specified path and decodes it into a slice of Cluster objects.
//
// Parameters:
//   - path: A string representing the file path to the JSON file.
//
// Returns:
//   - []Cluster: A slice of Cluster objects decoded from the JSON file.
//   - Error: An error if the file cannot be opened or the JSON cannot be decoded.
func LoadClusters(path string) ([]Cluster, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open cluster config: %w", err)
	}
	defer file.Close()

	var clusters []Cluster
	err = json.NewDecoder(file).Decode(&clusters)
	if err != nil {
		return nil, fmt.Errorf("decode cluster config: %w", err)
	}
	return clusters, nil
}

// SaveClusters encodes a slice of Cluster objects into JSON format and writes it to the specified file path.
//
// Parameters:
//   - path: A string representing the file path where the JSON data will be written.
//   - clusters: A slice of Cluster objects to be encoded into JSON.
//
// Returns:
//   - error: An error if the JSON cannot be marshaled or the file cannot be written.
func SaveClusters(path string, clusters []Cluster) error {
	data, err := json.MarshalIndent(clusters, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cluster config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

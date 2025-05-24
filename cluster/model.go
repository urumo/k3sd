package cluster

import "fmt"

// Cluster represents a cluster configuration, including its domain and associated workers.
//
// Fields:
//   - Domain: The domain name associated with the cluster.
//   - Gitea: A Gitea configuration object containing PostgreSQL credentials.
//   - Workers: A slice of Worker objects representing the workers in the cluster.
type Cluster struct {
	Worker              // Embeds the Worker struct, inheriting its fields and methods.
	Domain     string   `json:"domain"`     // The domain name associated with the cluster.
	Gitea      Gitea    `json:"gitea"`      // Gitea configuration for the cluster.
	PrivateNet bool     `json:"privateNet"` // Indicates if the cluster uses a private network.
	Workers    []Worker `json:"workers"`    // List of worker nodes in the cluster.
}

// Worker represents a worker node in the cluster.
//
// Fields:
//   - Address: The IP address or hostname of the worker node.
//   - User: The username used to connect to the worker node.
//   - Password: The password used to authenticate the connection to the worker node.
//   - NodeName: The name of the node in the cluster.
//   - Labels: The labels assigned to the node for identification or grouping.
//   - Done: A boolean indicating whether the worker setup is complete.
type Worker struct {
	Address  string            `json:"address"`  // IP address or hostname of the worker node.
	User     string            `json:"user"`     // Username for connecting to the worker node.
	Password string            `json:"password"` // Password for authenticating the connection.
	NodeName string            `json:"nodeName"` // Name of the node in the cluster.
	Labels   map[string]string `json:"labels"`   // Labels for identification or grouping.
	Done     bool              `json:"done"`     // Indicates if the worker setup is complete.
}

// Gitea represents the Gitea configuration for the cluster.
//
// Fields:
//   - Pg: PostgreSQL configuration for Gitea.
type Gitea struct {
	Pg Pg `json:"pg"` // PostgreSQL configuration for Gitea.
}

// Pg represents the PostgreSQL configuration.
//
// Fields:
//   - Username: The username for the PostgreSQL database.
//   - Password: The password for the PostgreSQL database.
//   - DbName: The name of the PostgreSQL database.
type Pg struct {
	Username string `json:"user"`     // Username for the PostgreSQL database.
	Password string `json:"password"` // Password for the PostgreSQL database.
	DbName   string `json:"db"`       // Name of the PostgreSQL database.
}

func (worker *Worker) GetLabels() string {
	labels := ""
	for k, v := range worker.Labels {
		labels += fmt.Sprintf("%s=%s,", k, v)
	}
	if len(labels) > 0 {
		labels = labels[:len(labels)-1]
	}
	return labels
}

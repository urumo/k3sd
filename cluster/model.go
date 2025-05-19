package cluster

// Cluster represents a cluster configuration, including its domain and associated workers.
//
// Fields:
//   - Domain: The domain name associated with the cluster.
//   - Workers: A slice of Worker objects representing the workers in the cluster.
type Cluster struct {
	Worker
	Domain  string   `json:"domain"`
	Workers []Worker `json:"workers"`
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
	Address  string `json:"address"`
	User     string `json:"user"`
	Password string `json:"password"`
	NodeName string `json:"nodeName"`
	Labels   string `json:"labels"`
	Done     bool   `json:"done"`
}

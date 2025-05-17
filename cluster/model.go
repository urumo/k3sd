package cluster

type Cluster struct {
	Worker
	Workers []Worker `json:"workers"`
}

type Worker struct {
	Address  string `json:"address"`
	User     string `json:"user"`
	Password string `json:"password"`
	NodeName string `json:"nodeName"`
	Labels   string `json:"labels"`
	Done     bool   `json:"done"`
}

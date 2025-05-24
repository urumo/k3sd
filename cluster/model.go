package cluster

import "fmt"

type Cluster struct {
	Worker
	Domain     string   `json:"domain"`
	Gitea      Gitea    `json:"gitea"`
	PrivateNet bool     `json:"privateNet"`
	Workers    []Worker `json:"workers"`
}
type Worker struct {
	Address  string            `json:"address"`
	User     string            `json:"user"`
	Password string            `json:"password"`
	NodeName string            `json:"nodeName"`
	Labels   map[string]string `json:"labels"`
	Done     bool              `json:"done"`
}
type Gitea struct {
	Pg Pg `json:"pg"`
}
type Pg struct {
	Username string `json:"user"`
	Password string `json:"password"`
	DbName   string `json:"db"`
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

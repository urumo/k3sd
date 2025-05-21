# K3SD - K3s Cluster Deployment Tool

K3SD is a command-line tool for creating, managing, and uninstalling K3s Kubernetes clusters across multiple machines.
It automates the deployment of K3s clusters with optional components like cert-manager, Traefik, Prometheus, and
Linkerd.

## Features

- Deploy K3s clusters with multiple worker nodes via SSH
- Cross-platform support: Linux (x86_64/arm64), macOS (Apple Silicon), and Windows (x86_64)
- Install and configure additional components:
    - cert-manager
    - Traefik (HTTP/3 enabled)
    - Prometheus stack
    - Gitea
    - Linkerd (including multi-cluster)
- Generate and manage kubeconfig files
- Uninstall clusters cleanly
- Display version information with `--version`

## Prerequisites

- `kubectl` - [Kubernetes CLI](https://kubernetes.io/docs/tasks/tools/)
- `linkerd` - [Linkerd CLI](https://linkerd.io/2.18/getting-started/#step-1-install-the-cli) (required for Linkerd installations)
- `step` - [Certificate management tool](https://smallstep.com/docs/step-cli/installation/) (required for Linkerd)
- `ssh` - SSH client for remote server access

## Installation

Download the appropriate binary for your platform from the [Releases](https://github.com/urumo/k3sd/releases) page.

```bash
# Example for Linux x86_64
curl -LO https://github.com/urumo/k3sd/releases/latest/download/k3sd-linux-amd64.tar.gz
tar -xzf k3sd-linux-amd64.tar.gz
chmod +x k3sd
sudo mv k3sd /usr/local/bin/
```

## Configuration

Create a JSON configuration file for your clusters. Example:

```json
[
  {
    "address": "192.168.1.10",
    "user": "root",
    "password": "password",
    "nodeName": "master-1",
    "labels": "node-role.kubernetes.io/control-plane=true",
    "domain": "example.com",
    "workers": [
      {
        "address": "192.168.1.11",
        "user": "root",
        "password": "password",
        "nodeName": "worker-1",
        "labels": "node-role.kubernetes.io/worker=true"
      }
    ]
  }
]
```

## Usage

### Display Version

```bash
k3sd --version
```

### Create a Cluster

```bash
k3sd --config-path=/path/to/clusters.json
```

### Create a Cluster with Additional Components

```bash
k3sd --config-path=/path/to/clusters.json \
  --cert-manager \
  --traefik \
  --cluster-issuer \
  --prometheus \
  --gitea
```

### Install Linkerd

```bash
k3sd --config-path=/path/to/clusters.json --linkerd
```

### Install Linkerd with Multi-cluster Support

```bash
k3sd --config-path=/path/to/clusters.json --linkerd-mc
```

### Uninstall a Cluster

```bash
k3sd --config-path=/path/to/clusters.json --uninstall
```

## Command-line Options

| Option             | Description                                           |
|--------------------|-------------------------------------------------------|
| `--config-path`    | Path to clusters.json (required)                      |
| `--cert-manager`   | Install cert-manager                                  |
| `--traefik`        | Install Traefik                                       |
| `--cluster-issuer` | Apply Cluster Issuer YAML (requires domain in config) |
| `--gitea`          | Install Gitea                                         |
| `--gitea-ingress`  | Apply Gitea Ingress (requires domain in config)       |
| `--prometheus`     | Install Prometheus stack                              |
| `--linkerd`        | Install Linkerd                                       |
| `--linkerd-mc`     | Install Linkerd with multi-cluster support            |
| `--uninstall`      | Uninstall the cluster                                 |
| `--version`        | Print the version and exit                            |

## Build from Source

```bash
git clone https://github.com/urumo/k3sd.git
cd k3sd
go build -ldflags "-X 'github.com/urumo/k3sd/utils.Version=<version>'" -o k3sd ./cli/main.go
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

```
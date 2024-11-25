# Hacox

English | [简体中文](README_zh.md)

hacox is the multi-apiserver proxy for Kubernetes clusters.

It is deployed on each node and proxies multiple kube-apiservers for the kubelet.

Hacox continuously gathers apiserver's endpoints from the cluster's nodes and pod information, enabling automatic updates to the configuration when control plane nodes change.

It also monitors the health of the apiservers and automatically removes unhealthy apiservers from the forwarding list.

```
Usage:
  hacox [flags]

Flags:
      --address strings                   the listen addresses (default [127.0.0.1:5443,[::1]:5443])
      --backend-port int                  the backend apiserver listening port (default 6443)
      --check-interval duration           the interval for checking the health of the backend apiservers (default 2s)
  -h, --help                              help for this command
      --kubeconfig string                 the Kubernetes client config path (default "$HOME/.kube/config")
      --metrics-addr string               the metrics listen address (default ":5444")
      --unhealthy-count-threshold int     the threshold for the number of unhealthy counts (default 3)
      --refresh-interval duration         the interval for refresh the backend apiserver addresses config from the Kubernetes cluster (default 2m0s)
      --servers-config string             the backend apiserver addresses config path (default "servers.yaml")
      --version                           show version
```

[hacox.yaml](deploy/hacox.yaml) is an example of deploying hacox using static pods.

The configuration file `servers.yaml` contains only the IP of the backend apiservers, without the port, as shown below:

```yaml
- 10.0.0.1
- 10.0.0.2
- 10.0.0.3
```

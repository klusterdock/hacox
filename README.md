# Hacox

Hacox is a multi-apiserver proxy.

Deployed on each node, it proxies multiple kube-apiservers for the kubelet. Hacox continuously gathers kube-apiserver endpoints from the cluster's nodes and pod information, enabling automatic updates to the configuration when kube-apiserver endpoints change. It also monitors the health of the kube-apiservers.

```
Usage:
  hacox [flags]

Flags:
      --address strings                   the listen addresses (default [127.0.0.1:5443,[::1]:5443])
      --backend-port int                  the backend kube-apiserver listening port (default 6443)
      --check-interval duration           the interval for checking the health of the backend servers (default 2s)
  -h, --help                              help for this command
      --kubeconfig string                 the kubernetes kubeconfig path (default "$HOME/.kube/config")
      --metrics-addr string               the metrics listen address (default ":5444")
      --unhealthy-count-threshold int     the threshold for the number of unhealthy counts (default 3)
      --refresh-interval duration         the interval for refresh the backend servers config from kubernetes (default 2m0s)
      --servers-config string             the backend servers config path (default "servers.yaml")
      --version                           show version
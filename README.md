# Hacox

Hacox is a configuration auto reloader for HAProxy.

It's deployed with HAProxy on each node. The HAProxy proxies the multiple kube-apiserver. While the Hacox continually collects the kube-apiserver's endpoints from the cluster's nodes and pods information. Hacox will auto-update HAProxy's configuration when the kube-apiserver's endpoint is changed.

```
Usage:
  hacox [flags]

Flags:
    --haproxy-config-template string   the haproxy config template path (default "/etc/hacox/haproxy.cfg.tmpl")
-h, --help                             help for this command
    --kube-config string               the kubeconfig path (default "/Users/wangtengfei/.kube/config")
    --listen-port int                  the listen port (default 5443)
    --refresh-interval duration        the interval for refresh the backend servers config (default 1m0s)
    --server-port int                  the backend server port (default 6443)
    --servers-config string            the backend servers config path (default "servers.yaml")
    --version                          show version
```
# Hacox

[English](README.md) | 简体中文

hacox 是 Kubernetes 集群的多 apiserver 代理。

它部署在每个节点上，为 kubelet 代理多个 apiserver。

hacox 持续从集群的节点和 pod 信息中收集 apiserver 的访问地址，以便在更换控制节点时能自动更新配置。

hacox 还监控 apiserver 的健康状况，并根据健康状况从转发列表中自动剔除不健康的 apiserver。

```
Usage:
  hacox [flags]

Flags:
      --address strings                   监听地址 (默认值 [127.0.0.1:5443,[::1]:5443])
      --backend-port int                  后端 apiserver 监听端口 (默认值 6443)
      --check-interval duration           检查后端 apiserver 健康状况的间隔时间 (默认值 2s)
  -h, --help                              查看帮助
      --kubeconfig string                 Kubernetes 的客户端配置文件路径 (默认值 $HOME/.kube/config)
      --metrics-addr string               metrics 监听地址 (默认值 ":5444")
      --unhealthy-count-threshold int     不健康次数阈值 (默认值 3)
      --refresh-interval duration         从 Kubernetes 集群更新 apiserver 地址配置的刷新时间间隔 (默认值 2m0s)
      --servers-config string             后端 apiserver 地址配置文件路径 (默认值 "servers.yaml")
      --version                           显示版本
```

[hacox.yaml](deploy/hacox.yaml) 是采用静态 Pod 部署 hacox 的示例。

配置文件 `servers.yaml` 中只包含后端 apiserver 的IP，不包含端口，示例如下：

```yaml
- 10.0.0.1
- 10.0.0.2
- 10.0.0.3
```

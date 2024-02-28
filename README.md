# hacox

Hacox is a configuration auto reloader for HAProxy.

It's deployed with HAProxy on each node. The HAProxy proxies the multiple kube-apiservers. While the Hacox continually collects the kube-apiserver's endpoints from the cluster's nodes and pods informations. Hacox will auto-update HAProxy's configuration when the kube-apiserver's endpoint is changed.

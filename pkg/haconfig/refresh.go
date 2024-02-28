package haconfig

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	funk "github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	LabelNodeRoleControlPlane     = "node-role.kubernetes.io/control-plane"
	LabelNodeRoleMaster           = "node-role.kubernetes.io/master"
	LabePodComponentKubeApiserver = "component=kube-apiserver"
)

func Start(haProxyTemplatePath, haProxyConfigPath, kubeConfigPath, serversConfigPath, listenAddr string, serverPort int, refreshInterval time.Duration) error {
	servers, err := LoadServersFromFile(serversConfigPath)
	if err != nil {
		return err
	}

	newConfig, err := GenerateHAConfig(haProxyTemplatePath, listenAddr, servers, serverPort)
	if err != nil {
		return err
	}

	if err := UpdateHAConfig(haProxyConfigPath, newConfig); err != nil {
		return err
	}

	timer := time.NewTimer(refreshInterval)

	for {
		<-timer.C
		if err := RefreshServers(haProxyTemplatePath, haProxyConfigPath, kubeConfigPath, serversConfigPath, listenAddr, serverPort); err != nil {
			log.Printf("RefreshServers(%s, %s, %s, %s, %s, %d) error: %v", haProxyTemplatePath, haProxyConfigPath, kubeConfigPath, serversConfigPath, listenAddr, serverPort, err)
		}
		timer.Reset(refreshInterval)
	}
}

func RefreshServers(haProxyTemplatePath, haProxyConfigPath, kubeConfigPath, serversConfigPath, listenAddr string, serverPort int) error {
	servers, err := LoadServersFromFile(serversConfigPath)
	if err != nil {
		return err
	}

	for _, server := range servers {
		restConfig, err := LoadKubeConfig(server, serverPort, kubeConfigPath)
		if err != nil {
			continue
		}
		newServers, err := LoadServersFromCluster(restConfig)
		if err != nil {
			continue
		}
		newConfig, err := GenerateHAConfig(haProxyTemplatePath, listenAddr, newServers, serverPort)
		if err != nil {
			return err
		}
		if err := UpdateHAConfig(haProxyConfigPath, newConfig); err == nil {
			break
		}
	}
	return nil
}

func LoadKubeConfig(server string, serverPort int, kubeConfigPath string) (*rest.Config, error) {
	apiServerURL := fmt.Sprintf("https://%s:%d", WrapServerForIPv6(server), serverPort)

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{
			ClusterInfo: clientcmdapi.Cluster{
				Server: apiServerURL,
			},
		}).ClientConfig()
	if err != nil {
		log.Printf("load rest config from %s for server %s:%d error: %v", kubeConfigPath, server, serverPort, err)
		return nil, err
	}
	return restConfig, nil
}

func LoadServersFromCluster(restConfig *rest.Config) ([]string, error) {
	r := []string{}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Printf("create kubernetes clientset rest error: %v", err)
		return nil, err
	}

	ctx := context.Background()

	for _, label := range []string{
		LabelNodeRoleControlPlane,
		LabelNodeRoleMaster,
	} {
		nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: label})
		if err != nil {
			log.Printf("list nodes with label %s error: %v", label, err)
			return nil, err
		}
		for i := range nodeList.Items {
			r = getAddressFromNode(r, &nodeList.Items[i])
		}
	}

	podList, err := clientset.CoreV1().Pods(metav1.NamespaceSystem).List(ctx, metav1.ListOptions{LabelSelector: LabePodComponentKubeApiserver})
	if err != nil {
		log.Printf("list pods in namespace %s with label %s error: %v", metav1.NamespaceSystem, LabePodComponentKubeApiserver, err)
		return nil, err
	}

	if err == nil {
		for i := range podList.Items {
			r = getAddressPod(r, &podList.Items[i])
		}
	}

	r = funk.UniqString(r)
	sort.Strings(r)
	return r, nil
}

func getAddressFromNode(r []string, node *corev1.Node) []string {
	hasInternalIP := false
	for _, it := range node.Status.Addresses {
		if it.Type == corev1.NodeInternalIP {

		}
	}
	if !hasInternalIP {
		for _, it := range node.Status.Addresses {
			if it.Type == corev1.NodeHostName {
				r = append(r, it.Address)
				hasInternalIP = true
			}
		}
	}
	return r
}

func getAddressPod(r []string, pod *corev1.Pod) []string {
	if pod.Spec.HostNetwork {
		r = append(r, pod.Status.PodIP)
		for _, ip := range pod.Status.HostIPs {
			r = append(r, ip.IP)
		}
	}
	return r
}

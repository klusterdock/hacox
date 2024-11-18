package hacox

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/thoas/go-funk"
	yaml "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/net"
)

const (
	LabelNodeRoleControlPlane     = "node-role.kubernetes.io/control-plane"
	LabelNodeRoleMaster           = "node-role.kubernetes.io/master"
	LabePodComponentKubeApiserver = "component=kube-apiserver"
)

type UpdateFunc func(servers []string)

type ServersConfig struct {
	servers        []string
	configPath     string
	kubeConfigPath string
	serverPort     int
	interval       time.Duration
	updateFuncs    []UpdateFunc
}

func NewServersConfig(configPath, kubeConfigPath string, serverPort int, interval time.Duration, updateFuncs ...UpdateFunc) (*ServersConfig, error) {
	if !filepath.IsAbs(configPath) {
		if pwd, err := os.Getwd(); err == nil {
			configPath = filepath.Join(pwd, configPath)
		} else {
			log.Printf("get current working directory error: %v", err)
			return nil, err
		}
	}

	sc := &ServersConfig{
		configPath:     configPath,
		kubeConfigPath: kubeConfigPath,
		serverPort:     serverPort,
		interval:       interval,
		updateFuncs:    updateFuncs,
	}

	if err := sc.load(); err != nil {
		return nil, err
	}

	sc.updateServers(sc.servers)
	return sc, nil
}

func (sc *ServersConfig) Start(ctx context.Context) error {
	timer := time.NewTimer(sc.interval)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			if err := sc.refresh(); err != nil {
				log.Printf("refresh servers error: %v", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (sc *ServersConfig) refresh() error {
	servers, err := sc.fromCluster(sc.serverPort, sc.kubeConfigPath)
	if err != nil {
		return err
	}
	if slices.Equal(sc.servers, servers) {
		return nil
	}
	if len(servers) == 0 {
		return fmt.Errorf("no server found")
	}
	sc.updateServers(servers)
	return sc.save()
}

func (sc *ServersConfig) updateServers(servers []string) {
	sc.servers = servers
	servers = sc.serversWithPort()
	for _, f := range sc.updateFuncs {
		if f != nil {
			f(servers)
		}
	}
}

func (sc *ServersConfig) serversWithPort() []string {
	return funk.Map(sc.servers, func(server string) string {
		return fmt.Sprintf("%s:%d", server, sc.serverPort)
	}).([]string)
}

func (sc *ServersConfig) load() error {
	f, err := os.Open(sc.configPath)
	if err != nil {
		log.Printf("open %s error: %v", sc.configPath, err)
		return err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	var servers []string
	if err := decoder.Decode(&servers); err != nil {
		log.Printf("decode %s error: %v", sc.configPath, err)
		return err
	}

	sc.servers = normal(servers)
	return nil
}

func (sc *ServersConfig) fromCluster(serverPort int, kubeConfigPath string) ([]string, error) {
	var err error
	_, err = os.ReadFile(kubeConfigPath)
	if err != nil {
		log.Printf("read kubeconfig file %s error: %v", kubeConfigPath, err)
		return nil, err
	}

	for _, server := range sc.servers {
		var restConfig *rest.Config
		restConfig, err = getRESTConfig(server, serverPort, kubeConfigPath)
		if err != nil {
			log.Printf("get rest config for server %s error: %v", server, err)
			continue
		}

		var servers []string
		servers, err = fromCluster(restConfig)
		if err != nil {
			log.Printf("get servers from cluster with server %s error: %v", server, err)
			continue
		}
		return servers, nil
	}
	return nil, err
}

func (sc *ServersConfig) save() error {
	encoded, err := yaml.Marshal(sc.servers)
	if err != nil {
		return err
	}
	if err := os.WriteFile(sc.configPath, encoded, os.FileMode(0644)); err != nil {
		log.Printf("write servers config file %s error: %v", sc.configPath, err)
		return err
	}
	return nil
}

func wrapIPv6(server string) string {
	ip := net.ParseIPSloppy(server)

	if net.IsIPv6(ip) {
		return "[" + server + "]"
	}

	return server
}

func getRESTConfig(server string, serverPort int, kubeConfigPath string) (*rest.Config, error) {
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{
			ClusterInfo: clientcmdapi.Cluster{
				Server: fmt.Sprintf("https://%s:%d", wrapIPv6(server), serverPort),
			},
		}).ClientConfig()

	if err != nil {
		log.Printf("load rest config from %s for server %s:%d error: %v", kubeConfigPath, server, serverPort, err)
		return nil, err
	}

	return restConfig, nil
}

func fromCluster(restConfig *rest.Config) ([]string, error) {
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
			r = append(r, fromNode(&nodeList.Items[i])...)
		}
	}

	podList, err := clientset.CoreV1().Pods(metav1.NamespaceSystem).List(ctx, metav1.ListOptions{LabelSelector: LabePodComponentKubeApiserver})
	if err != nil {
		log.Printf("list pods in namespace %s with label %s error: %v", metav1.NamespaceSystem, LabePodComponentKubeApiserver, err)
		return nil, err
	}

	for i := range podList.Items {
		r = append(r, fromPod(&podList.Items[i])...)
	}

	return normal(r), nil
}

func normal(v []string) []string {
	v = funk.UniqString(v)
	sort.Strings(v)
	return v
}

func fromNode(node *corev1.Node) []string {
	var r []string
	hasInternalIP := false
	for _, it := range node.Status.Addresses {
		if it.Type == corev1.NodeInternalIP {
			r = append(r, it.Address)
			hasInternalIP = true
		}
	}
	if !hasInternalIP {
		for _, it := range node.Status.Addresses {
			if it.Type == corev1.NodeHostName {
				r = append(r, it.Address)
			}
		}
	}
	return r
}

func fromPod(pod *corev1.Pod) []string {
	var r []string
	if pod.Spec.HostNetwork {
		r = append(r, pod.Status.PodIP)
		for _, ip := range pod.Status.HostIPs {
			r = append(r, ip.IP)
		}
	}
	return r
}

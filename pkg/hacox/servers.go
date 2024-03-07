package hacox

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/thoas/go-funk"
	yaml "gopkg.in/yaml.v3"
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

type ServersConfig struct {
	Servers []string `yaml:"servers"`
}

func (s *ServersConfig) Load(path string) error {
	if !filepath.IsAbs(path) {
		if pwd, err := os.Getwd(); err == nil {
			path = filepath.Join(pwd, path)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("open %s error: %v", path, err)
		return err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(s); err != nil {
		log.Printf("decode %s error: %v", path, err)
		return err
	}
	return nil
}

func (s *ServersConfig) NewFromCluster(serverPort int, kubeConfigPath string) *ServersConfig {
	for _, server := range s.Servers {
		restConfig, err := loadKubeConfig(server, serverPort, kubeConfigPath)
		if err != nil {
			continue
		}
		newServers, err := loadServersFromCluster(restConfig)
		if err != nil {
			continue
		}
		return &ServersConfig{Servers: newServers}
	}
	return s
}

func (s *ServersConfig) Save(path string) error {
	if !filepath.IsAbs(path) {
		if pwd, err := os.Getwd(); err == nil {
			path = filepath.Join(pwd, path)
		}
	}

	encoded, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, encoded, os.FileMode(0644)); err != nil {
		log.Printf("write %s error: %v", path, err)
		return err
	}
	return nil
}

func (s *ServersConfig) Equal(other *ServersConfig) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	if len(s.Servers) != len(other.Servers) {
		return false
	}
	for i := range s.Servers {
		if s.Servers[i] != other.Servers[i] {
			return false
		}
	}
	return true
}

func (s *ServersConfig) Update(path string) error {
	ns := ServersConfig{}
	if err := ns.Load(path); err != nil {
		return err
	}
	if s.Equal(&ns) {
		return nil
	}
	return s.Save(path)
}

func loadKubeConfig(server string, serverPort int, kubeConfigPath string) (*rest.Config, error) {
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

func loadServersFromCluster(restConfig *rest.Config) ([]string, error) {
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

	for i := range podList.Items {
		r = getAddressFromPod(r, &podList.Items[i])
	}

	r = funk.UniqString(r)
	sort.Strings(r)
	return r, nil
}

func getAddressFromNode(r []string, node *corev1.Node) []string {
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

func getAddressFromPod(r []string, pod *corev1.Pod) []string {
	if pod.Spec.HostNetwork {
		r = append(r, pod.Status.PodIP)
		for _, ip := range pod.Status.HostIPs {
			r = append(r, ip.IP)
		}
	}
	return r
}

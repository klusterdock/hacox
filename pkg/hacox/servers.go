package hacox

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/thoas/go-funk"
	yaml "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/net"
)

const (
	LabelNodeRoleControlPlane     = "node-role.kubernetes.io/control-plane"
	LabelNodeRoleMaster           = "node-role.kubernetes.io/master"
	LabePodComponentKubeApiserver = "component=kube-apiserver"
)

type UpdateFunc func(servers []string)

type ServersConfig struct {
	client         *http.Client
	servers        []string
	configPath     string
	kubeConfigPath string
	serverPort     int
	interval       time.Duration
	updateFuncs    []UpdateFunc
	authHeader     string
	clientCert     *tls.Certificate
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
	sc.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify:   true,
				GetClientCertificate: sc.getClientCertificate,
			},
		},
	}

	if err := sc.load(); err != nil {
		return nil, err
	}

	sc.updateServers(sc.servers)
	return sc, nil
}

func (sc *ServersConfig) Start(ctx context.Context) error {
	_ = sc.refresh()
	timer := time.NewTimer(sc.interval)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			if err := sc.refresh(); err != nil {
				log.Printf("refresh servers error: %v", err)
			}
			timer.Reset(sc.interval)
		case <-ctx.Done():
			return nil
		}
	}
}

func (sc *ServersConfig) refresh() error {
	servers, err := sc.fromCluster()
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

func (sc *ServersConfig) fromCluster() ([]string, error) {
	var err error
	if err := sc.prepareAuthConfig(); err != nil {
		log.Printf("prepare auth config error: %v", err)
		return nil, err
	}

	for _, server := range disorder(sc.servers) {
		var servers []string
		servers, err = sc.fetchFromCluster(server, sc.serverPort)
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

func (sc *ServersConfig) getClientCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return sc.clientCert, nil
}

func (sc *ServersConfig) prepareAuthConfig() error {
	cfg, err := clientcmd.LoadFromFile(sc.kubeConfigPath)
	if err != nil {
		log.Printf("load kubeconfig file %s error: %v", sc.kubeConfigPath, err)
		return err
	}

	if len(cfg.Contexts) == 0 {
		return fmt.Errorf("no context found in kubeconfig file %s", sc.kubeConfigPath)
	}

	context := cfg.Contexts[cfg.CurrentContext]
	if context == nil {
		return fmt.Errorf("no context named '%s' found in kubeconfig file %s", cfg.CurrentContext, sc.kubeConfigPath)
	}

	authInfo := cfg.AuthInfos[context.AuthInfo]
	if authInfo == nil {
		return fmt.Errorf("no auth info named '%s' found in context %s", context.AuthInfo, cfg.CurrentContext)
	}

	if authInfo.Token != "" {
		sc.authHeader = "Bearer " + authInfo.Token
		sc.clientCert = nil
		return nil
	}

	if authInfo.TokenFile != "" {
		token, err := os.ReadFile(authInfo.TokenFile)
		if err != nil {
			return fmt.Errorf("read token file %s error: %v", authInfo.TokenFile, err)
		}
		sc.authHeader = "Bearer " + string(token)
		sc.clientCert = nil
		return nil
	}

	if authInfo.Username != "" && authInfo.Password != "" {
		sc.authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(authInfo.Username+":"+authInfo.Password))
		sc.clientCert = nil
		return nil
	}

	if len(authInfo.ClientCertificateData) > 0 && len(authInfo.ClientKeyData) > 0 {
		cert, err := tls.X509KeyPair(authInfo.ClientCertificateData, authInfo.ClientKeyData)
		if err != nil {
			return fmt.Errorf("parse client certificate error: %v", err)
		}
		sc.clientCert = &cert
		sc.authHeader = ""
		return nil
	}

	if authInfo.ClientCertificate != "" && authInfo.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(authInfo.ClientCertificate, authInfo.ClientKey)
		if err != nil {
			return fmt.Errorf("load client certificate from file %s and key file %s error: %v", authInfo.ClientCertificate, authInfo.ClientKey, err)
		}
		sc.clientCert = &cert
		sc.authHeader = ""
		return nil
	}

	if authInfo.Exec != nil {
		return fmt.Errorf("exec auth info is not supported")
	}

	if authInfo.AuthProvider != nil {
		return fmt.Errorf("auth provider is not supported")
	}

	return nil
}

type Node struct {
	Status struct {
		Addresses []corev1.NodeAddress `json:"addresses"`
	} `json:"status"`
}

type NodeList struct {
	Items []Node `json:"items"`
}

type Pod struct {
	Spec struct {
		HostNetwork bool `json:"hostNetwork"`
	} `json:"spec"`
	Status struct {
		PodIP   string          `json:"podIP"`
		HostIPs []corev1.HostIP `json:"hostIPs"`
	} `json:"status"`
}

type PodList struct {
	Items []Pod `json:"items"`
}

func (sc *ServersConfig) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if sc.authHeader != "" {
		req.Header.Set("Authorization", sc.authHeader)
	}

	return sc.client.Do(req)
}

func (sc *ServersConfig) fetchFromCluster(server string, serverPort int) ([]string, error) {
	r := []string{}

	for _, label := range []string{
		LabelNodeRoleControlPlane,
		LabelNodeRoleMaster,
	} {
		url := fmt.Sprintf("https://%s:%d/api/v1/nodes?labelSelector=%s", wrapIPv6(server), serverPort, label)

		resp, err := sc.get(url)
		if err != nil {
			log.Printf("get nodes from %s error: %v", url, err)
			return nil, err
		}
		defer resp.Body.Close()

		t, err := fromNodes(resp.Body)
		if err != nil {
			log.Printf("decode nodes from %s error: %v", url, err)
			return nil, err
		}
		r = append(r, t...)
	}

	url := fmt.Sprintf("https://%s:%d/api/v1/namespaces/%s/pods?labelSelector=%s", wrapIPv6(server), serverPort, metav1.NamespaceSystem, LabePodComponentKubeApiserver)

	resp, err := sc.get(url)
	if err != nil {
		log.Printf("get pods from %s error: %v", url, err)
		return nil, err
	}
	defer resp.Body.Close()

	t, err := fromPod(resp.Body)
	if err != nil {
		log.Printf("decode pods from %s error: %v", url, err)
		return nil, err
	}
	r = append(r, t...)

	return normal(r), nil
}

func normal(v []string) []string {
	v = funk.UniqString(v)
	sort.Strings(v)
	return v
}

func disorder(v []string) []string {
	rand.Shuffle(len(v), func(i, j int) {
		v[i], v[j] = v[j], v[i]
	})
	return v
}

func wrapIPv6(server string) string {
	ip := net.ParseIPSloppy(server)

	if net.IsIPv6(ip) {
		return "[" + server + "]"
	}

	return server
}

func fromNodes(data io.ReadCloser) ([]string, error) {
	var r []string
	var nodeList NodeList
	if err := json.NewDecoder(data).Decode(&nodeList); err != nil {
		return nil, err
	}
	for _, node := range nodeList.Items {
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
	}
	return r, nil
}

func fromPod(data io.ReadCloser) ([]string, error) {
	var r []string
	var podList PodList
	if err := json.NewDecoder(data).Decode(&podList); err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.HostNetwork {
			r = append(r, pod.Status.PodIP)
			for _, ip := range pod.Status.HostIPs {
				r = append(r, ip.IP)
			}
		}
	}
	return r, nil
}

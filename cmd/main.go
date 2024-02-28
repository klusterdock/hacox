package main

import (
	"fmt"
	"hacox/pkg/haconfig"
	"hacox/version"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	flags := pflag.NewFlagSet("haproxy-config-reloader", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := NewRootCommand(flags)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func NewRootCommand(flags *pflag.FlagSet) *cobra.Command {
	var (
		haProxyConfig   string
		haProxyTemplate string
		kubeConfig      string
		listenAddr      string
		serversConfig   string
		serverPort      int
		refreshInterval time.Duration
		showVersion     bool
	)

	defaultKubeConfig := filepath.Join(".kube", "config")
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		defaultKubeConfig = filepath.Join(homeDir, defaultKubeConfig)
	}

	flags.StringVar(&haProxyConfig, "haproxy-config", "/etc/haproxy/haproxy.cfg", "the haproxy config path")
	flags.StringVar(&haProxyTemplate, "haproxy-config-template", "haproxy.cfg.tmpl", "the haproxy config template path")
	flags.StringVar(&kubeConfig, "kube-config", defaultKubeConfig, "the kubeconfig path")
	flags.StringVar(&listenAddr, "listen", "::1:5443", "the listen address")
	flags.StringVar(&serversConfig, "servers-config", "servers.yaml", "the backend servers config path")
	flags.IntVar(&serverPort, "server-port", 6443, "the backend server port")
	flags.DurationVar(&refreshInterval, "refresh-interval", time.Minute, "the interval for refresh the backend servers config")
	flags.BoolVar(&showVersion, "version", false, "show version")

	cmd := &cobra.Command{
		Short: "HAProxy config reloader",
		Long:  "Auto refresh HAProxy config for the Kubernetes controlplane endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println(version.BuildVersion)
				return nil
			}
			return haconfig.Start(haProxyTemplate, haProxyConfig, kubeConfig, serversConfig, listenAddr, serverPort, refreshInterval)
		},
	}

	return cmd
}

package main

import (
	"fmt"
	"hacox/pkg/hacox"
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
	flags := pflag.NewFlagSet("config", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := NewRootCommand(flags)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func NewRootCommand(flags *pflag.FlagSet) *cobra.Command {
	var (
		kubeConfig              string
		serversConfig           string
		listenAddrs             []string
		backendPort             int
		unHealthyCountThreshold int
		checkInterval           time.Duration
		refreshInterval         time.Duration
		showVersion             bool
		metricsAddr             string
	)

	defaultKubeConfig := filepath.Join(".kube", "config")
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		defaultKubeConfig = filepath.Join(homeDir, defaultKubeConfig)
	}

	flags.StringSliceVar(&listenAddrs, "address", []string{"127.0.0.1:5443", "[::1]:5443"}, "the listen addresses")
	flags.IntVar(&backendPort, "backend-port", 6443, "the backend apiserver listening port")
	flags.DurationVar(&checkInterval, "check-interval", 2*time.Second, "the interval for checking the health of the backend apiservers")
	flags.StringVar(&kubeConfig, "kubeconfig", defaultKubeConfig, "the Kubernetes client config path")
	flags.StringVar(&metricsAddr, "metrics-addr", ":5444", "the metrics listen address")
	flags.IntVar(&unHealthyCountThreshold, "unhealthy-count-threshold", 3, "the threshold for the number of unhealthy counts")
	flags.DurationVar(&refreshInterval, "refresh-interval", 2*time.Minute, "the interval for refresh the backend apiserver addresses config from the Kubernetes cluster")
	flags.StringVar(&serversConfig, "servers-config", "servers.yaml", "the backend apiserver addresses config path")
	flags.BoolVar(&showVersion, "version", false, "show version")

	cmd := &cobra.Command{
		Short: "proxy multiple Kubernetes apiservers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println(version.BuildVersion)
				return nil
			}
			return hacox.Start(
				kubeConfig,
				serversConfig,
				metricsAddr,
				listenAddrs,
				backendPort,
				unHealthyCountThreshold,
				checkInterval,
				refreshInterval,
			)
		},
	}

	return cmd
}

package hacox

import (
	"context"
	"log"
	"strings"
	"time"
)

func Start(kubeConfigPath, serversConfigPath, metricsAddr string, listenAddrs []string, backendPort, notHealthyCountThreshold int, checkInterval, refreshInterval time.Duration) error {

	log.Printf("starting hacox on %s", strings.Join(listenAddrs, ", "))
	log.Printf("not healthy count threshold: %d", notHealthyCountThreshold)
	log.Printf("refresh interval: %s", refreshInterval)
	log.Printf("check interval: %s", checkInterval)
	log.Printf("kubeconfig path: %s", kubeConfigPath)
	log.Printf("servers config path: %s", serversConfigPath)
	log.Printf("backend port: %d", backendPort)
	log.Printf("metrics addr: %s", metricsAddr)

	proxy := NewProxy(listenAddrs)
	hc := NewHealthCheck(checkInterval, notHealthyCountThreshold, proxy.OnNotify)
	metrics := NewMetrics(metricsAddr, proxy.GetBackendsClientsCount, hc.GetBackendsHealth)

	sc, err := NewServersConfig(serversConfigPath, kubeConfigPath, backendPort, refreshInterval, proxy.UpdateBackends, hc.UpdateBackends)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = metrics.Start(ctx)
		if err != nil {
			log.Printf("start metrics server error: %v", err)
		}
		cancel()
	}()

	go func() {
		err = hc.Start(ctx)
		if err != nil {
			log.Printf("start health check error: %v", err)
		}
		cancel()
	}()

	go func() {
		err = sc.Start(ctx)
		if err != nil {
			log.Printf("start servers config error: %v", err)
		}
		cancel()
	}()

	go func() {
		err = proxy.Start(ctx)
		if err != nil {
			log.Printf("start proxy error: %v", err)
		}
		cancel()
	}()

	log.Println("hacox started")
	<-ctx.Done()
	log.Println("hacox stopped")
	return err
}

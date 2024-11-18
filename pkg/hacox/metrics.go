package hacox

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	descBackendsCount  = prometheus.NewDesc("hacox_backends_count", "The number of backends", nil, nil)
	descBackendsHealth = prometheus.NewDesc("hacox_backends_health", "The health of backends", []string{"backend"}, nil)
	descClientsCount   = prometheus.NewDesc("hacox_clients_count", "The number of connected clients", []string{"backend"}, nil)
)

type GetClientsCountFunc func() map[string]int
type GetHealthyFunc func() map[string]bool

type Metrics struct {
	metricsAddr         string
	getClientsCountFunc GetClientsCountFunc
	getHealthyFunc      GetHealthyFunc
	registry            *prometheus.Registry
}

func NewMetrics(metricsAddr string, getClientsCountFunc GetClientsCountFunc, getHealthyFunc GetHealthyFunc) *Metrics {
	m := &Metrics{
		metricsAddr:         metricsAddr,
		getClientsCountFunc: getClientsCountFunc,
		getHealthyFunc:      getHealthyFunc,
		registry:            prometheus.NewRegistry(),
	}
	m.registry.MustRegister(m)
	return m
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- descBackendsCount
	ch <- descBackendsHealth
	ch <- descClientsCount
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	if m.getClientsCountFunc == nil || m.getHealthyFunc == nil {
		return
	}

	for backend, count := range m.getClientsCountFunc() {
		ch <- prometheus.MustNewConstMetric(descClientsCount, prometheus.GaugeValue, float64(count), backend)
	}

	n := 0
	for backend, healthy := range m.getHealthyFunc() {
		n++
		ch <- prometheus.MustNewConstMetric(descBackendsHealth, prometheus.GaugeValue, boolToFloat64(healthy), backend)
	}

	ch <- prometheus.MustNewConstMetric(descBackendsCount, prometheus.GaugeValue, float64(n))

}

func (m *Metrics) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	server := &http.Server{Addr: m.metricsAddr, Handler: mux}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

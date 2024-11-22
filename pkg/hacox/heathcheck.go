package hacox

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"
)

const (
	HealthCheckPath = "/readyz"
)

type NotifyFunc func(backend string, healthy bool)

type HealthCheck struct {
	client                  *http.Client
	lock                    sync.RWMutex
	backends                []string
	checkInterval           time.Duration
	checking                map[string]struct{}
	unHealthyCount          map[string]int
	unHealthyCountThreshold int
	isHealthy               map[string]bool
	notiftyFunc             NotifyFunc
}

func NewHealthCheck(checkInterval time.Duration, unHealthyCountThreshold int, notifyfunc NotifyFunc) *HealthCheck {
	return &HealthCheck{
		checkInterval:           checkInterval,
		unHealthyCountThreshold: unHealthyCountThreshold,
		checking:                make(map[string]struct{}),
		unHealthyCount:          make(map[string]int),
		isHealthy:               make(map[string]bool),
		notiftyFunc:             notifyfunc,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func (hc *HealthCheck) GetBackendsHealth() map[string]bool {
	hc.lock.RLock()
	defer hc.lock.RUnlock()

	health := make(map[string]bool)
	for backend, healthy := range hc.isHealthy {
		health[backend] = healthy
	}

	return health
}

func (hc *HealthCheck) Start(ctx context.Context) error {
	timer := time.NewTimer(hc.checkInterval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			for _, backend := range hc.backends {
				if err := hc.check(backend); err != nil {
					log.Printf("health check failed: %s", err)
				}
			}
			timer.Reset(hc.checkInterval)
		case <-ctx.Done():
			return nil
		}
	}
}

func (hc *HealthCheck) check(backend string) error {
	hc.lock.Lock()
	if _, ok := hc.checking[backend]; ok {
		hc.lock.Unlock()
		return nil
	}
	hc.checking[backend] = struct{}{}
	hc.lock.Unlock()

	var err error
	defer func() {
		hc.updateStatue(backend, err)
	}()

	resp, err := hc.client.Get(fmt.Sprintf("https://%s%s", backend, HealthCheckPath))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("health check failed: %s", resp.Status)
		return err
	}

	return nil
}

func (hc *HealthCheck) updateStatue(backend string, err error) {
	var healthy, stateChanged bool

	if err != nil {
		stateChanged = hc.failed(backend)
		healthy = false
		if stateChanged {
			log.Printf("health check %s failed: %s", backend, err)
		}
	} else {
		stateChanged = hc.success(backend)
		healthy = true
		if stateChanged {
			log.Printf("health check %s success", backend)
		}
	}

	if stateChanged && hc.notiftyFunc != nil {
		hc.notiftyFunc(backend, healthy)
	}
}

func (hc *HealthCheck) failed(backend string) bool {
	hc.lock.Lock()
	defer hc.lock.Unlock()

	delete(hc.checking, backend)
	if hc.isHealthy[backend] {
		hc.unHealthyCount[backend]++
	}
	if hc.unHealthyCount[backend] >= hc.unHealthyCountThreshold {
		if hc.isHealthy[backend] {
			hc.isHealthy[backend] = false
			return true
		}
	}
	return false
}

func (hc *HealthCheck) success(backend string) bool {
	hc.lock.Lock()
	defer hc.lock.Unlock()

	delete(hc.checking, backend)
	hc.unHealthyCount[backend] = 0
	if !hc.isHealthy[backend] {
		hc.isHealthy[backend] = true
		return true
	}
	return false
}

func (hc *HealthCheck) UpdateBackends(backends []string) {
	var oldBackends []string

	hc.lock.Lock()
	if slices.Equal(hc.backends, backends) {
		hc.lock.Unlock()
		return
	}
	oldBackends = slices.Clone(hc.backends)
	hc.backends = slices.Clone(backends)
	hc.lock.Unlock()

	for _, it := range backends {
		if !slices.Contains(oldBackends, it) {
			hc.isHealthy[it] = true
		}
	}

	var removed []string
	for _, it := range oldBackends {
		if !slices.Contains(backends, it) {
			removed = append(removed, it)
		}
	}

	for _, it := range removed {
		hc.lock.Lock()
		delete(hc.isHealthy, it)
		delete(hc.unHealthyCount, it)
		hc.lock.Unlock()
		if hc.notiftyFunc != nil {
			hc.notiftyFunc(it, false)
		}
	}
}

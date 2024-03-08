package hacox

import (
	"strings"
	"testing"
)

var testServers = []string{
	"192.168.1.1",
	"a.b.com",
	"fe80::200:ff:fe7e:12d",
}

var testHAProxyConfig = strings.TrimSpace(`
global
  log stdout format raw local0 info
  maxconn 51200

defaults
  mode tcp
  maxconn 51200
  option redispatch
  option abortonclose
  timeout connect 5000ms
  timeout client 2m
  timeout server 2m
  log global
  balance roundrobin

frontend proxy-ipv4
  bind 127.0.0.1:5443
  mode tcp
  default_backend backends

frontend proxy-ipv6
  bind ::1:5443
  mode tcp
  default_backend backends

backend backends
  mode tcp
  balance roundrobin
  default-server on-marked-down shutdown-sessions

  option httpchk
  http-check connect ssl alpn h2,http/1.1
  http-check send meth GET uri /healthz
  http-check expect status 200

  server s0 192.168.1.1:6443 check port 6443 inter 1000 maxconn 51200 verify none
  server s1 a.b.com:6443 check port 6443 inter 1000 maxconn 51200 verify none
  server s2 [fe80::200:ff:fe7e:12d]:6443 check port 6443 inter 1000 maxconn 51200 verify none
`)

func TestRender(t *testing.T) {
	cfg, err := (&HaConfig{
		ListenPort: 5443,
		ServerPort: 6443,
		Servers:    testServers,
	}).Render("../../haproxy.cfg.tmpl")
	if err != nil {
		t.Logf("Test HaConfig.Render error: %v", err)
		t.Fail()
	} else {
		if cfg != testHAProxyConfig {
			t.Logf("Test HaConfig.Render \nexpect: \n%s \ngot: \n%s", testHAProxyConfig, cfg)
			t.Fail()
		}
	}
}

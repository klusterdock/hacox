package hacox

import (
	"k8s.io/utils/net"
)

func WrapServerForIPv6(server string) string {
	ip := net.ParseIPSloppy(server)
	if net.IsIPv6(ip) {
		return "[" + server + "]"
	}

	return server
}

func WrapServersForIPv6(servers []string) []string {
	n := len(servers)
	r := make([]string, n)
	for i := range servers {
		r[i] = WrapServerForIPv6(servers[i])
	}
	return r
}

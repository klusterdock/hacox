package hacox

import (
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

const (
	HaproxyProgramName = "haproxy"
	HaproxyConfigPath  = "/tmp/haproxy.cfg"
	defaultHaproxyUid  = 99
	defaultHaproxyGid  = 99
	haproxyUsername    = "haproxy"
	haproxyGroupname   = "haproxy"
	haproxyWorkingDir  = "/var/lib/haproxy"
)

type Hacox struct {
	pid                 int
	listenPort          int
	serverPort          int
	kubeConfigPath      string
	haProxyTemplatePath string
	serversConfigPath   string
	haProxyConfigPath   string
}

func Start(haProxyTemplatePath, kubeConfigPath, serversConfigPath string, listenPort, serverPort int, refreshInterval time.Duration) error {
	h := Hacox{
		listenPort:          listenPort,
		serverPort:          serverPort,
		kubeConfigPath:      kubeConfigPath,
		haProxyTemplatePath: haProxyTemplatePath,
		serversConfigPath:   serversConfigPath,
		haProxyConfigPath:   HaproxyConfigPath,
	}

	if err := h.generate(); err != nil {
		return err
	}

	if err := h.startHAProxy(); err != nil {
		return err
	}

	timer := time.NewTimer(refreshInterval)

	for {
		<-timer.C
		if err := h.refresh(); err != nil {
			log.Printf("refresh error: %v", err)
		}
		timer.Reset(refreshInterval)
	}
}

func (h *Hacox) startHAProxy() error {
	cmdPath, err := exec.LookPath(HaproxyProgramName)
	if err != nil {
		return err
	}
	cmd := exec.Command(cmdPath, "-W", "-db", "-f", h.haProxyConfigPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	uid := defaultHaproxyUid
	gid := defaultHaproxyGid

	if u, err := user.Lookup(haproxyUsername); err == nil {
		if v, err := strconv.Atoi(u.Gid); err == nil {
			uid = v
		}
	}

	if g, err := user.LookupGroup(haproxyGroupname); err == nil {
		if v, err := strconv.Atoi(g.Gid); err == nil {
			gid = v
		}
	}

	_ = os.MkdirAll(haproxyWorkingDir, os.FileMode(0755))
	_ = os.Chown(haproxyWorkingDir, uid, gid)

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	cmd.Dir = haproxyWorkingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	go func() {
		log.Fatal(cmd.Wait())
	}()

	h.pid = cmd.Process.Pid
	return nil
}

func (h *Hacox) signalHAProxyReload() error {
	p, err := os.FindProcess(h.pid)
	if err != nil {
		log.Printf(`find process %d error: %v`, h.pid, err)
		return err
	}

	if err := p.Signal(syscall.SIGHUP); err != nil {
		log.Printf(`send hup signal to %d error: %v`, h.pid, err)
		return err
	}
	return nil
}

func (h *Hacox) generate() error {
	sc := &ServersConfig{}
	if err := sc.Load(h.serversConfigPath); err != nil {
		return err
	}

	hc := &HaConfig{
		ListenPort: h.listenPort,
		ServerPort: h.serverPort,
		Servers:    sc.Servers,
	}

	newConfig, err := hc.Render(h.haProxyTemplatePath)
	if err != nil {
		return err
	}

	if _, err = hc.Update(newConfig, h.haProxyConfigPath); err != nil {
		return err
	}
	return nil
}

func (h *Hacox) refresh() error {
	sc := &ServersConfig{}
	if err := sc.Load(h.serversConfigPath); err != nil {
		return err
	}

	sc = sc.NewFromCluster(h.serverPort, h.kubeConfigPath)
	if err := sc.Update(h.serversConfigPath); err != nil {
		return err
	}

	hc := &HaConfig{
		ListenPort: h.listenPort,
		ServerPort: h.serverPort,
		Servers:    sc.Servers,
	}
	newConfig, err := hc.Render(h.haProxyTemplatePath)
	if err != nil {
		return err
	}

	changed, err := hc.Update(newConfig, h.haProxyConfigPath)
	if err != nil {
		return err
	}

	if changed {
		if err := h.signalHAProxyReload(); err != nil {
			return err
		}
	}

	return nil
}

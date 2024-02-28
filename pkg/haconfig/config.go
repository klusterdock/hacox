package haconfig

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	ps "github.com/mitchellh/go-ps"
	yaml "gopkg.in/yaml.v3"
)

const (
	haproxyProgramName = "haproxy"
)

type ServersConfig struct {
	Servers []string `yaml:"servers"`
}

type HaConfig struct {
	Listen     string
	ServerPort int
	Servers    []string
}

func LoadServersFromFile(serversConfigPath string) ([]string, error) {
	if !filepath.IsAbs(serversConfigPath) {
		if pwd, err := os.Getwd(); err == nil {
			serversConfigPath = filepath.Join(pwd, serversConfigPath)
		}
	}

	f, err := os.Open(serversConfigPath)
	if err != nil {
		log.Printf("open %s error: %v", serversConfigPath, err)
		return nil, err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	r := ServersConfig{}
	if err := decoder.Decode(&r); err != nil {
		log.Printf("decode %s error: %v", serversConfigPath, err)
		return nil, err
	}
	return r.Servers, nil
}

func SignalHAProxyReload() error {
	processes, err := ps.Processes()
	if err != nil {
		log.Printf("ps.Processes() error: %v", err)
		return err
	}

	haproxyPid := -1
	for _, proc := range processes {
		cmd := proc.Executable()
		if cmd != "" && strings.Contains(cmd, haproxyProgramName) {
			haproxyPid = proc.Pid()
			break
		}
	}

	if haproxyPid < 0 {
		log.Printf(`connot find process with name "%s"`, haproxyProgramName)
		return nil
	}

	p, err := os.FindProcess(haproxyPid)
	if err != nil {
		log.Printf(`find process %d error: %v`, haproxyPid, err)
		return err
	}

	if err := p.Signal(syscall.SIGHUP); err != nil {
		log.Printf(`send hup signal to %d error: %v`, haproxyPid, err)
		return err
	}
	return nil
}

func GenerateHAConfig(haProxyTemplatePath, listenAddr string, servers []string, serverPort int) (string, error) {
	if !filepath.IsAbs(haProxyTemplatePath) {
		if pwd, err := os.Getwd(); err == nil {
			haProxyTemplatePath = filepath.Join(pwd, haProxyTemplatePath)
		}
	}

	tmplstr, err := os.ReadFile(haProxyTemplatePath)
	if err != nil {
		log.Printf("open %s error: %v", haProxyTemplatePath, err)
		return "", err
	}

	tmpl, err := template.New("haproxy-config").Parse(string(tmplstr))
	if err != nil {
		log.Printf("parse haproxy config template error: %v", err)
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, &HaConfig{
		Listen:     listenAddr,
		ServerPort: serverPort,
		Servers:    WrapServersForIPv6(servers),
	}); err != nil {
		log.Printf("render haproxy config template error: %v", err)
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func UpdateHAConfig(haProxyConfigPath string, newConfig string) error {
	if !filepath.IsAbs(haProxyConfigPath) {
		if pwd, err := os.Getwd(); err == nil {
			haProxyConfigPath = filepath.Join(pwd, haProxyConfigPath)
		}
	}

	currentConfig, err := os.ReadFile(haProxyConfigPath)
	if err != nil {
		log.Printf("open %s error: %v", haProxyConfigPath, err)
		if !os.IsNotExist(err) {
			return err
		}
	}
	if string(currentConfig) != newConfig {
		log.Printf("%s need update", haProxyConfigPath)
		if err := os.WriteFile(haProxyConfigPath, []byte(newConfig), os.FileMode(0644)); err != nil {
			log.Printf("write %s error: %v", haProxyConfigPath, err)
			return err
		}
		if err := SignalHAProxyReload(); err != nil {
			return err
		}
	}
	return nil
}

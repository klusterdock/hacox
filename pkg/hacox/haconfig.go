package hacox

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type HaConfig struct {
	ListenPort int
	ServerPort int
	Servers    []string
}

func (c *HaConfig) Render(templPath string) (string, error) {
	if !filepath.IsAbs(templPath) {
		if pwd, err := os.Getwd(); err == nil {
			templPath = filepath.Join(pwd, templPath)
		}
	}

	tmplstr, err := os.ReadFile(templPath)
	if err != nil {
		log.Printf("open %s error: %v", templPath, err)
		return "", err
	}

	tmpl, err := template.New("haproxy-config").Parse(string(tmplstr))
	if err != nil {
		log.Printf("parse haproxy config template error: %v", err)
		return "", err
	}
	c.Servers = WrapServersForIPv6(c.Servers)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, c); err != nil {
		log.Printf("render haproxy config template error: %v", err)
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func (c *HaConfig) Update(newConfig, path string) (bool, error) {
	currentConfig, err := os.ReadFile(path)
	if err != nil {
		log.Printf("open %s error: %v", path, err)
		if !os.IsNotExist(err) {
			return false, err
		}
	}
	if string(currentConfig) == newConfig {
		return false, nil
	}

	log.Printf("%s need update", path)
	if err := os.WriteFile(path, []byte(newConfig), os.FileMode(0644)); err != nil {
		log.Printf("write %s error: %v", path, err)
		return false, err
	}
	return true, nil
}

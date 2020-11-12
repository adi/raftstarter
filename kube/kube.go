package kube

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	hclog "github.com/hashicorp/go-hclog"
)

func loadStringFromFile(fileName string) (string, error) {
	contentsBytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("Couldn't read file contents from %s: %w", fileName, err)
	}
	contents := string(contentsBytes)
	return strings.TrimSpace(contents), nil
}

// RunningInKube tries to guess if we're running in Kubernetes
func RunningInKube() bool {
	markerFile := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	fi, err := os.Stat(markerFile)
	if os.IsNotExist(err) || fi.IsDir() {
		return false
	}
	return true
}

// SetSelfRaftState updates the raft state in kubernetes
func SetSelfRaftState(raftState string, logger hclog.Logger) error {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	payloadBytes, err := json.Marshal([]map[string]string{{
		"op":    "replace",
		"path":  "/metadata/labels/raft-state",
		"value": raftState,
	}})
	if err != nil {
		return fmt.Errorf("Cannot encode JSON: %w", err)
	}

	me, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Cannot get own hostname: %w", err)
	}

	token, err := loadStringFromFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return fmt.Errorf("Cannot read token: %w", err)
	}

	namespace, err := loadStringFromFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return fmt.Errorf("Cannot read current namespace: %w", err)
	}

	host := "127.0.0.1"
	if envVar := os.Getenv("KUBERNETES_SERVICE_HOST"); envVar != "" {
		host = envVar
	}

	port := "443"
	if envVar := os.Getenv("KUBERNETES_PORT_443_TCP_PORT"); envVar != "" {
		port = envVar
	}

	url := fmt.Sprintf("https://%s:%s/api/v1/namespaces/%s/pods/%s", host, port, namespace, me)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("Cannot create patch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json-patch+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Cannot patch pod: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Cannot patch pod: %v", resp.StatusCode)
	}
	defer resp.Body.Close()

	logger.Debug("synced raft state to kubernetes", "resp", resp)

	return nil
}

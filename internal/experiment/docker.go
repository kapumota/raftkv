package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type NodeStatus struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	Term        int    `json:"term"`
	CommitIndex int    `json:"commit_index"`
}

type backend interface {
	Statuses(context.Context) ([]NodeStatus, error)
	Prepare(context.Context) (map[string]string, error)
	Apply(context.Context, string, string) error
	Restore(context.Context, string, string) error
	Write(context.Context, string, string, string) (int, error)
}

type endpoint struct {
	IPAddress string
	Aliases   []string
}

type dockerBackend struct {
	client    *http.Client
	urls      map[string]string
	network   string
	endpoints map[string]endpoint
}

func newDockerBackend() *dockerBackend {
	d := &dockerBackend{client: &http.Client{Timeout: 7 * time.Second}, urls: map[string]string{}, endpoints: map[string]endpoint{}}
	for i := 1; i <= 5; i++ {
		d.urls[fmt.Sprintf("raft-node-%d", i)] = fmt.Sprintf("http://127.0.0.1:%d", 18080+i)
	}
	return d
}

func dockerCommand(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("falló Docker (%s): %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (d *dockerBackend) Statuses(ctx context.Context) ([]NodeStatus, error) {
	var statuses []NodeStatus
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("raft-node-%d", i)
		call, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		req, _ := http.NewRequestWithContext(call, http.MethodGet, d.urls[id]+"/status", nil)
		resp, err := d.client.Do(req)
		if err == nil {
			var status NodeStatus
			err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&status)
			resp.Body.Close()
			if err == nil && resp.StatusCode == http.StatusOK && status.ID == id {
				statuses = append(statuses, status)
			}
		}
		cancel()
	}
	if len(statuses) == 0 {
		return nil, fmt.Errorf("ningún nodo respondió al estado")
	}
	return statuses, nil
}

type containerInfo struct {
	Image           string
	State           struct{ Running bool }
	NetworkSettings struct{ Networks map[string]endpoint }
}

func inspectContainer(ctx context.Context, id string) (containerInfo, error) {
	var infos []containerInfo
	data, err := dockerCommand(ctx, "inspect", id)
	if err != nil {
		return containerInfo{}, err
	}
	if err := json.Unmarshal(data, &infos); err != nil || len(infos) != 1 {
		return containerInfo{}, fmt.Errorf("inspección inválida de %s", id)
	}
	return infos[0], nil
}

func (d *dockerBackend) Prepare(ctx context.Context) (map[string]string, error) {
	metadata := map[string]string{}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("raft-node-%d", i)
		info, err := inspectContainer(ctx, id)
		if err != nil {
			return nil, err
		}
		if !info.State.Running || len(info.NetworkSettings.Networks) != 1 {
			return nil, fmt.Errorf("%s debe estar iniciado en una sola red", id)
		}
		for network, ep := range info.NetworkSettings.Networks {
			if d.network != "" && d.network != network {
				return nil, fmt.Errorf("los nodos no comparten una red única")
			}
			d.network = network
			d.endpoints[id] = ep
		}
		hashes, err := dockerCommand(ctx, "exec", id, "sha256sum", "/data/log.jsonl", "/data/state.json")
		if err != nil {
			return nil, fmt.Errorf("no se pudo registrar el WAL inicial de %s: %w", id, err)
		}
		metadata[id] = "imagen=" + info.Image + "; red=" + d.network + "; WAL=" + string(hashes)
	}
	return metadata, nil
}

func (d *dockerBackend) Apply(ctx context.Context, kind, id string) error {
	if kind == "caida" {
		_, err := dockerCommand(ctx, "kill", "--signal=KILL", id)
		return err
	}
	_, err := dockerCommand(ctx, "network", "disconnect", d.network, id)
	return err
}

func (d *dockerBackend) Restore(ctx context.Context, kind, id string) error {
	info, err := inspectContainer(ctx, id)
	if err != nil {
		return err
	}
	if kind == "caida" {
		if info.State.Running {
			return nil
		}
		_, err = dockerCommand(ctx, "start", id)
		return err
	}
	if _, connected := info.NetworkSettings.Networks[d.network]; connected {
		return nil
	}
	ep := d.endpoints[id]
	args := []string{"network", "connect", "--ip", ep.IPAddress}
	for _, alias := range ep.Aliases {
		args = append(args, "--alias", alias)
	}
	args = append(args, d.network, id)
	_, err = dockerCommand(ctx, args...)
	return err
}

func (d *dockerBackend) Write(ctx context.Context, id, key, value string) (int, error) {
	url, ok := d.urls[id]
	if !ok {
		return 0, fmt.Errorf("nodo desconocido: %s", id)
	}
	data, _ := json.Marshal(map[string]string{"key": key, "value": value})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/kv/set", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&body); err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusOK && body["estado"] != "confirmado" {
		return resp.StatusCode, fmt.Errorf("respuesta de confirmación inválida")
	}
	return resp.StatusCode, nil
}

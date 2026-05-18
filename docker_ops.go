package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DockerContainer represents a Docker container
type DockerContainer struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Status string            `json:"status"`
	Ports  []string          `json:"ports"`
	Labels map[string]string `json:"labels"`
}

// dockerList returns all Docker containers (running and stopped)
func dockerList() ([]DockerContainer, error) {
	// Use docker ps -a with custom format for parsing
	cmd := exec.Command("sudo", "docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %v\n%s", err, string(output))
	}

	if len(output) == 0 {
		return []DockerContainer{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	containers := make([]DockerContainer, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}

		container := DockerContainer{
			ID:     parts[0],
			Name:   parts[1],
			Image:  parts[2],
			Status: parts[3],
		}

		if len(parts) > 4 && parts[4] != "" {
			container.Ports = strings.Split(parts[4], ",")
		}

		// Get labels via docker inspect
		container.Labels = dockerGetLabels(container.ID)

		containers = append(containers, container)
	}

	return containers, nil
}

// dockerGetLabels retrieves labels for a container via docker inspect
func dockerGetLabels(containerID string) map[string]string {
	cmd := exec.Command("sudo", "docker", "inspect", "--format", "{{json .Config.Labels}}", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]string{}
	}

	var labels map[string]string
	if err := json.Unmarshal(output, &labels); err != nil {
		return map[string]string{}
	}

	return labels
}

// dockerStart starts a stopped container
func dockerStart(containerID string) error {
	cmd := exec.Command("sudo", "docker", "start", containerID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker start failed: %v\n%s", err, string(output))
	}
	return nil
}

// dockerStop stops a running container
func dockerStop(containerID string) error {
	cmd := exec.Command("sudo", "docker", "stop", containerID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop failed: %v\n%s", err, string(output))
	}
	return nil
}

// dockerRestart restarts a container
func dockerRestart(containerID string) error {
	cmd := exec.Command("sudo", "docker", "restart", containerID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker restart failed: %v\n%s", err, string(output))
	}
	return nil
}

// dockerStatus returns detailed status for a specific container
func dockerStatus(containerID string) (*DockerContainer, error) {
	containers, err := dockerList()
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if c.ID == containerID || c.Name == containerID {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("container '%s' not found", containerID)
}

// dockerLogs returns recent logs for a container (optional feature)
func dockerLogs(containerID string, tail int) (string, error) {
	cmd := exec.Command("sudo", "docker", "logs", "--tail", fmt.Sprintf("%d", tail), containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs failed: %v\n%s", err, string(output))
	}
	return string(output), nil
}

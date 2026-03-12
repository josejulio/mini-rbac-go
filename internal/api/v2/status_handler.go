package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redhat/mini-rbac-go/internal/infrastructure"
)

// StatusHandler handles status and health check requests
type StatusHandler struct {
	config    *infrastructure.Config
	startTime time.Time
	commit    string
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(config *infrastructure.Config, commit string) *StatusHandler {
	return &StatusHandler{
		config:    config,
		startTime: time.Now(),
		commit:    commit,
	}
}

// StatusResponse represents the status response
type StatusResponse struct {
	Status      string `json:"status"`
	Application string `json:"application"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	Commit      string `json:"commit,omitempty"`
	Uptime      string `json:"uptime"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Healthy bool   `json:"healthy"`
}

// GetStatus handles GET /api/status
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(h.startTime)

	response := StatusResponse{
		Status:      "running",
		Application: h.config.App.Name,
		Version:     h.config.App.Version,
		Environment: h.config.App.Env,
		Commit:      h.commit,
		Uptime:      formatUptime(uptime),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetHealth handles GET /health
func (h *StatusHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:  "healthy",
		Healthy: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// formatUptime formats duration into human-readable uptime
func formatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return formatDuration(days, "day", hours, "hour", minutes, "minute")
	}
	if hours > 0 {
		return formatDuration(hours, "hour", minutes, "minute", seconds, "second")
	}
	if minutes > 0 {
		return formatDuration(minutes, "minute", seconds, "second", 0, "")
	}
	return formatDuration(seconds, "second", 0, "", 0, "")
}

func formatDuration(v1 int, u1 string, v2 int, u2 string, v3 int, u3 string) string {
	result := formatUnit(v1, u1)
	if v2 > 0 && u2 != "" {
		result += " " + formatUnit(v2, u2)
	}
	if v3 > 0 && u3 != "" {
		result += " " + formatUnit(v3, u3)
	}
	return result
}

func formatUnit(value int, unit string) string {
	if value == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", value, unit)
}

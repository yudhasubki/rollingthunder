package database

import "context"

const (
	ConnectionHealthUnknown      = "unknown"
	ConnectionHealthHealthy      = "healthy"
	ConnectionHealthDegraded     = "degraded"
	ConnectionHealthReconnecting = "reconnecting"
)

type ConnectionHealth struct {
	ConnectionID string `json:"connectionId"`
	State        string `json:"state"`
	Message      string `json:"message,omitempty"`
	LatencyMS    int64  `json:"latencyMs"`
	FailureCount int    `json:"failureCount"`
	LastChecked  string `json:"lastChecked,omitempty"`
	LastHealthy  string `json:"lastHealthy,omitempty"`
}

type HealthDriver interface {
	Ping(ctx context.Context) error
}

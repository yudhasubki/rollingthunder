package db

import "rollingthunder/pkg/database"

type ConnectRequest struct {
	Driver string          `json:"driver"`
	Config database.Config `json:"config"`
}

type ConnectResponse struct {
	Connected    bool   `json:"connected"`
	ConnectionID string `json:"connectionId,omitempty"`
}

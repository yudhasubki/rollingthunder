package db

import "time"

// Operational timeouts live in one policy file so database workflows cannot
// silently drift to different, undocumented magic values.
const (
	defaultServiceVersion        = "0.0.1"
	defaultConnectionTimeout     = 15 * time.Second
	defaultHealthMonitorInterval = 20 * time.Second
	defaultHealthCheckTimeout    = 5 * time.Second
	healthMonitorShutdownTimeout = 2 * time.Second
	activityTimeout              = 10 * time.Second
	objectMetadataTimeout        = 20 * time.Second
	explainQueryTimeout          = 30 * time.Second
	restoreRollbackTimeout       = 30 * time.Second
	objectChangeTimeout          = 60 * time.Second
)

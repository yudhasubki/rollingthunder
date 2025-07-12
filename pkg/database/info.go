package database

type Info struct {
	Engine   string `json:"engine"`
	Version  string `json:"version"`
	Database string `json:"database"`
}

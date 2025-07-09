package db

type ConnectRequest struct {
	Driver string `json:"driver"`
	Config Config `json:"config"`
}

type ConnectResponse struct {
	Connected bool `json:"connected"`
}

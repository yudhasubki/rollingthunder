package db

type Driver interface {
	Connect() error
	Close() error
	GetCollections(schema ...string) ([]string, error)
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

type Config struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

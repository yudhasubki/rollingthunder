package database

type Driver interface {
	Connect() error
	Close() error
	GetCollections(schema ...string) ([]string, error)
	GetCollectionStructures(schema, table string) (Structures, error)
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

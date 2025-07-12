package database

type Driver interface {
	Connect() error
	Close() error
	GetCollections(schema ...string) ([]string, error)
	GetCollectionStructures(schema, table string) (Structures, error)
	GetIndices(schema, table string) (Indices, error)
	GetDatabaseInfo() (Info, error)
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

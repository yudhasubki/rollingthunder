package database

type Driver interface {
	Connect() error
	Close() error
	CountCollectionData(table Table) (int, error)
	GetCollectionData(table Table) (Structures, []map[string]interface{}, error)
	GetCollections(schema ...string) ([]string, error)
	GetCollectionStructures(table Table) (Structures, error)
	GetIndices(table Table) (Indices, error)
	GetDatabaseInfo() (Info, error)
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

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
	// CRUD operations
	InsertRow(table Table, data map[string]interface{}) error
	UpdateRow(table Table, data map[string]interface{}, primaryKey string) error
	DeleteRow(table Table, primaryKey string, primaryValue interface{}) error
	ExecuteQuery(query string) ([]map[string]interface{}, error)
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

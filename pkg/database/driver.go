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
	// Table management
	CreateTable(table Table, columns []ColumnDefinition) error
	DropTable(table Table) error
	TruncateTable(table Table) error
	GetDataTypes() []DataType
}

type DriverWithSchema interface {
	GetSchemas() ([]string, error)
}

// ColumnDefinition for creating tables
type ColumnDefinition struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	Default    string `json:"default"`
	PrimaryKey bool   `json:"primaryKey"`
	Unique     bool   `json:"unique"`
}

// DataType info for UI
type DataType struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

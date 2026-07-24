package database

const DefaultImportPreviewRows = 50

type ImportFileSelection struct {
	Token  string `json:"token"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Size   int64  `json:"size"`
}

type ImportOptions struct {
	Format      string `json:"format"`
	Delimiter   string `json:"delimiter,omitempty"`
	Header      bool   `json:"header"`
	EmptyAsNull bool   `json:"emptyAsNull"`
}

type ImportPreviewRequest struct {
	Token   string        `json:"token"`
	Options ImportOptions `json:"options"`
	Limit   int           `json:"limit,omitempty"`
}

type ImportColumn struct {
	SourceName   string `json:"sourceName"`
	TargetName   string `json:"targetName"`
	InferredType string `json:"inferredType"`
	Nullable     bool   `json:"nullable"`
	Included     bool   `json:"included"`
}

type ImportPreview struct {
	File    ImportFileSelection      `json:"file"`
	Columns []ImportColumn           `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
	Sampled int                      `json:"sampled"`
}

type ImportRequest struct {
	ConnectionID string         `json:"connectionId"`
	Token        string         `json:"token"`
	Options      ImportOptions  `json:"options"`
	Schema       string         `json:"schema"`
	Table        string         `json:"table"`
	CreateTable  bool           `json:"createTable"`
	Columns      []ImportColumn `json:"columns"`
}

type ImportResult struct {
	Schema       string   `json:"schema"`
	Table        string   `json:"table"`
	RowsInserted int      `json:"rowsInserted"`
	TableCreated bool     `json:"tableCreated"`
	Warnings     []string `json:"warnings"`
}

package db

import (
	"context"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

type Service struct {
	ctx    context.Context
	driver database.Driver
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Start(ctx context.Context) {
	s.ctx = ctx
}

func (s *Service) Connect(req ConnectRequest) response.BaseResponse[ConnectResponse] {
	driver, err := NewDriver(s.ctx, req.Driver, req.Config)
	if err != nil {
		return response.BaseResponse[ConnectResponse]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: ConnectResponse{
				Connected: false,
			},
		}
	}

	err = driver.Connect()
	if err != nil {
		return response.BaseResponse[ConnectResponse]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: ConnectResponse{
				Connected: false,
			},
		}
	}

	s.driver = driver

	return response.BaseResponse[ConnectResponse]{
		Data: ConnectResponse{
			Connected: true,
		},
	}
}

func (s *Service) GetCollections(schema []string) response.BaseResponse[[]string] {
	collections, err := s.driver.GetCollections(schema...)
	if err != nil {
		return response.BaseResponse[[]string]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[[]string]{
		Data: collections,
	}
}

func (s *Service) GetCollectionStructures(table database.Table) response.BaseResponse[database.Structures] {
	structures, err := s.driver.GetCollectionStructures(table)
	if err != nil {
		return response.BaseResponse[database.Structures]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Structures]{
		Data: structures,
	}
}

func (s *Service) GetIndices(table database.Table) response.BaseResponse[database.Indices] {
	indices, err := s.driver.GetIndices(table)
	if err != nil {
		return response.BaseResponse[database.Indices]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Indices]{
		Data: indices,
	}
}

func (s *Service) GetSchemas() response.BaseResponse[[]string] {
	if d, ok := s.driver.(database.DriverWithSchema); ok {
		schemas, err := d.GetSchemas()
		if err != nil {
			return response.BaseResponse[[]string]{
				Errors: []response.BaseErrorResponse{
					{
						Detail: err.Error(),
					},
				},
			}
		}

		return response.BaseResponse[[]string]{
			Data: schemas,
		}
	}

	return response.BaseResponse[[]string]{
		Data: []string{},
	}
}

func (s *Service) GetDatabaseInfo() response.BaseResponse[database.Info] {
	info, err := s.driver.GetDatabaseInfo()
	if err != nil {
		return response.BaseResponse[database.Info]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Info]{
		Data: info,
	}
}

func (s *Service) CountCollectionData(table database.Table) response.BaseResponse[int] {
	count, err := s.driver.CountCollectionData(table)
	if err != nil {
		return response.BaseResponse[int]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[int]{
		Data: count,
	}
}

func (s *Service) GetCollectionData(table database.Table) response.BaseResponse[database.TableData] {
	structures, results, err := s.driver.GetCollectionData(table)
	if err != nil {
		return response.BaseResponse[database.TableData]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	resp := response.BaseResponse[database.TableData]{
		Data: database.TableData{
			Structures: make(database.Structures, 0),
			Data:       make([]map[string]interface{}, 0),
		},
	}
	if len(structures) > 0 {
		resp.Data.Structures = structures
	}

	if len(results) > 0 {
		resp.Data.Data = results
	}

	return resp
}

// InsertRow inserts a new row into the table
func (s *Service) InsertRow(table database.Table, data map[string]interface{}) response.BaseResponse[bool] {
	err := s.driver.InsertRow(table, data)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// UpdateRow updates an existing row in the table
func (s *Service) UpdateRow(table database.Table, data map[string]interface{}, primaryKey string) response.BaseResponse[bool] {
	err := s.driver.UpdateRow(table, data, primaryKey)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// DeleteRow deletes a row from the table
func (s *Service) DeleteRow(table database.Table, primaryKey string, primaryValue interface{}) response.BaseResponse[bool] {
	err := s.driver.DeleteRow(table, primaryKey, primaryValue)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// ExecuteQuery executes a raw SQL query
func (s *Service) ExecuteQuery(query string) response.BaseResponse[[]map[string]interface{}] {
	results, err := s.driver.ExecuteQuery(query)
	if err != nil {
		return response.BaseResponse[[]map[string]interface{}]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[[]map[string]interface{}]{
		Data: results,
	}
}

// CreateTable creates a new table in the database
func (s *Service) CreateTable(table database.Table, columns []database.ColumnDefinition) response.BaseResponse[bool] {
	err := s.driver.CreateTable(table, columns)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetDataTypes returns available data types for the current database driver
func (s *Service) GetDataTypes() response.BaseResponse[[]database.DataType] {
	types := s.driver.GetDataTypes()
	return response.BaseResponse[[]database.DataType]{
		Data: types,
	}
}

// DropTable drops a table from the database
func (s *Service) DropTable(table database.Table) response.BaseResponse[bool] {
	err := s.driver.DropTable(table)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// TruncateTable removes all rows from a table
func (s *Service) TruncateTable(table database.Table) response.BaseResponse[bool] {
	err := s.driver.TruncateTable(table)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

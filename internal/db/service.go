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

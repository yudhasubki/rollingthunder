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

func (s *Service) GetCollectionStructures(schema, table string) response.BaseResponse[database.Structures] {
	structures, err := s.driver.GetCollectionStructures(schema, table)
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

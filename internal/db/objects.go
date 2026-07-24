package db

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

const objectMetadataTimeout = 20 * time.Second

func (s *Service) GetCapabilities(
	connectionID string,
) response.BaseResponse[database.Capabilities] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceErrorWithCode[database.Capabilities](
			http.StatusNotFound,
			errorCodeDatabaseOperationFailed,
			"Connection unavailable",
			err.Error(),
			"Reconnect the database and try again.",
		)
	}
	defer release()

	capabilities := driver.Capabilities()
	if err := capabilities.Validate(); err != nil {
		return serviceErrorWithCode[database.Capabilities](
			http.StatusInternalServerError,
			errorCodeCapabilityInvalid,
			"Invalid driver capability contract",
			err.Error(),
			"Update the database driver before exposing its object workflows.",
		)
	}
	return response.BaseResponse[database.Capabilities]{Data: capabilities}
}

func (s *Service) objectContext() (context.Context, context.CancelFunc) {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, objectMetadataTimeout)
}

func validateObjectFilter(filter database.ObjectFilter) error {
	for _, kind := range filter.Kinds {
		if !kind.Valid() || kind == database.ObjectKindUnknown {
			return fmt.Errorf("unsupported database object kind %q", kind)
		}
	}
	if len(strings.TrimSpace(filter.Search)) > 256 {
		return fmt.Errorf("database object search is too long")
	}
	return nil
}

func (s *Service) GetDatabaseObjects(
	connectionID string,
	filter database.ObjectFilter,
) response.BaseResponse[[]database.DatabaseObject] {
	if err := validateObjectFilter(filter); err != nil {
		return serviceErrorWithCode[[]database.DatabaseObject](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid object filter",
			err.Error(),
			"Choose a supported object type and shorten the search text.",
		)
	}

	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[[]database.DatabaseObject](err.Error())
	}
	defer release()

	objectDriver, ok := driver.(database.ObjectDriver)
	if !ok {
		return serviceErrorWithCode[[]database.DatabaseObject](
			http.StatusNotImplemented,
			errorCodeObjectMetadataUnsupported,
			"Object explorer unavailable",
			"The connected driver does not expose normalized database objects.",
			"Use table browsing for this connection or update the driver.",
		)
	}

	ctx, cancel := s.objectContext()
	defer cancel()
	objects, err := objectDriver.ListObjects(ctx, filter)
	if err != nil {
		return serviceErrorWithCode[[]database.DatabaseObject](
			http.StatusBadRequest,
			errorCodeObjectMetadataFailed,
			"Could not load database objects",
			err.Error(),
			"Refresh the selected namespace or check metadata permissions.",
		)
	}
	if objects == nil {
		objects = make([]database.DatabaseObject, 0)
	}
	return response.BaseResponse[[]database.DatabaseObject]{Data: objects}
}

func (s *Service) GetDatabaseObject(
	connectionID string,
	reference database.ObjectReference,
) response.BaseResponse[database.ObjectDetail] {
	if err := reference.Validate(); err != nil {
		return serviceErrorWithCode[database.ObjectDetail](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid database object",
			err.Error(),
			"Refresh the object explorer and select the object again.",
		)
	}

	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.ObjectDetail](err.Error())
	}
	defer release()

	objectDriver, ok := driver.(database.ObjectDriver)
	if !ok {
		return serviceErrorWithCode[database.ObjectDetail](
			http.StatusNotImplemented,
			errorCodeObjectMetadataUnsupported,
			"Object details unavailable",
			"The connected driver cannot inspect this object.",
			"Use a SQL query to inspect the object or update the driver.",
		)
	}

	ctx, cancel := s.objectContext()
	defer cancel()
	detail, err := objectDriver.GetObjectDetail(ctx, reference)
	if err != nil {
		return serviceErrorWithCode[database.ObjectDetail](
			http.StatusBadRequest,
			errorCodeObjectMetadataFailed,
			"Could not inspect database object",
			err.Error(),
			"Refresh the object explorer and verify metadata permissions.",
		)
	}
	return response.BaseResponse[database.ObjectDetail]{Data: detail}
}

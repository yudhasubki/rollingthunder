package db

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

func (s *Service) structuralChangeContext() (context.Context, context.CancelFunc) {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, objectChangeTimeout)
}

func unsupportedObjectChange[T any]() response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		http.StatusNotImplemented,
		errorCodeObjectChangeUnsupported,
		"Structural change unavailable",
		"The connected driver does not support reviewed structural changes.",
		"Use a query tab to review and execute engine-specific DDL manually.",
	)
}

func (s *Service) PreviewDatabaseObjectChange(
	connectionID string,
	request database.ObjectChangeRequest,
) response.BaseResponse[database.ObjectChangePreview] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.ObjectChangePreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid structural change",
			err.Error(),
			"Complete the required object fields before generating a SQL preview.",
		)
	}

	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.ObjectChangePreview](err.Error())
	}
	defer release()
	changeDriver, ok := driver.(database.ObjectChangeDriver)
	if !ok {
		return unsupportedObjectChange[database.ObjectChangePreview]()
	}

	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	plan, err := changeDriver.BuildObjectChange(ctx, request)
	if err != nil {
		return serviceErrorWithCode[database.ObjectChangePreview](
			http.StatusBadRequest,
			errorCodeObjectChangePreviewFailed,
			"Could not generate SQL preview",
			err.Error(),
			"Review the object name and engine-specific definition.",
		)
	}
	preview, err := database.PreviewObjectChange(
		driver.Capabilities().Engine,
		plan,
	)
	if err != nil {
		return serviceErrorWithCode[database.ObjectChangePreview](
			http.StatusInternalServerError,
			errorCodeObjectChangePreviewFailed,
			"Invalid structural change plan",
			err.Error(),
			"The driver refused to expose an unsafe or incomplete SQL plan.",
		)
	}
	return response.BaseResponse[database.ObjectChangePreview]{Data: preview}
}

func reviewedFingerprintMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" || len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func (s *Service) ApplyDatabaseObjectChange(
	connectionID string,
	request database.ApplyObjectChangeRequest,
) response.BaseResponse[database.ObjectChangeResult] {
	if err := request.Change.Validate(); err != nil {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid structural change",
			err.Error(),
			"Return to the change editor and generate a fresh SQL preview.",
		)
	}
	if strings.TrimSpace(request.Fingerprint) == "" {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusConflict,
			errorCodeObjectChangeReviewRequired,
			"SQL review required",
			"The structural change has not been reviewed.",
			"Generate and review the SQL preview before applying it.",
		)
	}

	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.ObjectChangeResult](err.Error())
	}
	defer release()
	changeDriver, ok := driver.(database.ObjectChangeDriver)
	if !ok {
		return unsupportedObjectChange[database.ObjectChangeResult]()
	}

	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	plan, err := changeDriver.BuildObjectChange(ctx, request.Change)
	if err != nil {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusBadRequest,
			errorCodeObjectChangePreviewFailed,
			"Could not regenerate SQL plan",
			err.Error(),
			"Return to the change editor and generate a fresh SQL preview.",
		)
	}
	preview, err := database.PreviewObjectChange(
		driver.Capabilities().Engine,
		plan,
	)
	if err != nil {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusInternalServerError,
			errorCodeObjectChangePreviewFailed,
			"Invalid structural change plan",
			err.Error(),
			"Do not apply the change until the driver can generate a valid plan.",
		)
	}
	if !reviewedFingerprintMatches(request.Fingerprint, preview.Fingerprint) {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusConflict,
			errorCodeObjectChangeReviewRequired,
			"SQL preview changed",
			"The generated SQL no longer matches the reviewed preview.",
			"Review the refreshed SQL preview before applying the structural change.",
		)
	}

	if err := changeDriver.ApplyObjectChange(ctx, plan); err != nil {
		return serviceErrorWithCode[database.ObjectChangeResult](
			http.StatusBadRequest,
			errorCodeObjectChangeFailed,
			"Structural change failed",
			err.Error(),
			"The object list was not updated. Inspect database errors and review the SQL again.",
		)
	}
	return response.BaseResponse[database.ObjectChangeResult]{
		Data: database.ObjectChangeResult{
			Applied:        true,
			StatementCount: preview.StatementCount,
			Fingerprint:    preview.Fingerprint,
			Refresh:        preview.Refresh,
		},
	}
}

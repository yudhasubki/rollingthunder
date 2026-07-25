package db

import (
	"net/http"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

func unsupportedSecurity[T any]() response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		http.StatusNotImplemented,
		errorCodeSecurityUnsupported,
		"Security management unavailable",
		"The connected engine does not expose database roles or users.",
		"SQLite security is managed through operating-system file permissions.",
	)
}

func (s *Service) GetSecurityOverview(
	connectionID string,
	principal string,
	host string,
) response.BaseResponse[database.SecurityOverview] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.SecurityOverview](err.Error())
	}
	defer release()
	securityDriver, ok := driver.(database.SecurityDriver)
	if !ok {
		return response.BaseResponse[database.SecurityOverview]{
			Data: database.SecurityOverview{
				Supported:  false,
				Engine:     driver.Capabilities().Engine,
				Principals: []database.DatabasePrincipal{},
				Grants:     []database.DatabaseGrant{},
				Message:    "This engine uses file or server-level access controls instead of database principals.",
			},
		}
	}
	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	overview, err := securityDriver.GetSecurityOverview(
		ctx,
		strings.TrimSpace(principal),
		strings.TrimSpace(host),
	)
	if err != nil {
		return serviceErrorWithCode[database.SecurityOverview](
			http.StatusForbidden,
			errorCodeSecurityFailed,
			"Could not load users and grants",
			err.Error(),
			"Connect with an account allowed to inspect database roles and grants.",
		)
	}
	if overview.Principals == nil {
		overview.Principals = []database.DatabasePrincipal{}
	}
	if overview.Grants == nil {
		overview.Grants = []database.DatabaseGrant{}
	}
	return response.BaseResponse[database.SecurityOverview]{Data: overview}
}

func (s *Service) PreviewSecurityChange(
	connectionID string,
	request database.SecurityChangeRequest,
) response.BaseResponse[database.SecurityChangePreview] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.SecurityChangePreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid security change",
			err.Error(),
			"Complete the role, account, and privilege fields before previewing.",
		)
	}
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.SecurityChangePreview](err.Error())
	}
	defer release()
	securityDriver, ok := driver.(database.SecurityDriver)
	if !ok {
		return unsupportedSecurity[database.SecurityChangePreview]()
	}
	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	plan, err := securityDriver.BuildSecurityChange(ctx, request)
	if err != nil {
		return serviceErrorWithCode[database.SecurityChangePreview](
			http.StatusBadRequest,
			errorCodeSecurityFailed,
			"Could not generate security SQL",
			err.Error(),
			"Review the account name, scope, and engine-specific privilege.",
		)
	}
	if err := plan.Validate(); err != nil {
		return serviceErrorWithCode[database.SecurityChangePreview](
			http.StatusInternalServerError,
			errorCodeSecurityFailed,
			"Invalid security plan",
			err.Error(),
			"Do not apply this change until the driver can produce a complete plan.",
		)
	}
	warnings := plan.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return response.BaseResponse[database.SecurityChangePreview]{
		Data: database.SecurityChangePreview{
			Summary:        plan.Summary,
			SQL:            strings.Join(plan.PreviewStatements, "\n\n"),
			StatementCount: len(plan.Statements),
			Destructive:    plan.Destructive,
			Transactional:  plan.Transactional,
			Warnings:       warnings,
			Fingerprint:    plan.Fingerprint(driver.Capabilities().Engine),
		},
	}
}

func (s *Service) ApplySecurityChange(
	connectionID string,
	request database.ApplySecurityChangeRequest,
) response.BaseResponse[database.SecurityChangeResult] {
	if err := request.Change.Validate(); err != nil {
		return serviceErrorWithCode[database.SecurityChangeResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid security change",
			err.Error(),
			"Return to the security editor and generate a fresh preview.",
		)
	}
	if strings.TrimSpace(request.Fingerprint) == "" {
		return serviceErrorWithCode[database.SecurityChangeResult](
			http.StatusConflict,
			errorCodeSecurityReview,
			"Security review required",
			"The security SQL has not been reviewed.",
			"Generate and review the redacted SQL preview before applying it.",
		)
	}
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.SecurityChangeResult](err.Error())
	}
	defer release()
	securityDriver, ok := driver.(database.SecurityDriver)
	if !ok {
		return unsupportedSecurity[database.SecurityChangeResult]()
	}
	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	plan, err := securityDriver.BuildSecurityChange(ctx, request.Change)
	if err != nil {
		return serviceErrorWithCode[database.SecurityChangeResult](
			http.StatusBadRequest,
			errorCodeSecurityFailed,
			"Could not refresh security plan",
			err.Error(),
			"Generate a fresh preview before applying the change.",
		)
	}
	if err := plan.Validate(); err != nil {
		return serviceError[database.SecurityChangeResult](err.Error())
	}
	fingerprint := plan.Fingerprint(driver.Capabilities().Engine)
	if !reviewedFingerprintMatches(request.Fingerprint, fingerprint) {
		return serviceErrorWithCode[database.SecurityChangeResult](
			http.StatusConflict,
			errorCodeSecurityReview,
			"Security preview changed",
			"The account or privilege plan no longer matches the reviewed SQL.",
			"Review the refreshed SQL before applying the security change.",
		)
	}
	if err := securityDriver.ApplySecurityChange(ctx, plan); err != nil {
		return serviceErrorWithCode[database.SecurityChangeResult](
			http.StatusForbidden,
			errorCodeSecurityFailed,
			"Security change failed",
			err.Error(),
			"Verify that the connected account may manage users and grants.",
		)
	}
	return response.BaseResponse[database.SecurityChangeResult]{
		Data: database.SecurityChangeResult{
			Applied:        true,
			StatementCount: len(plan.Statements),
			Fingerprint:    fingerprint,
		},
	}
}

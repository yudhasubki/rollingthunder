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

const explainQueryTimeout = 30 * time.Second

func (s *Service) ExplainQuery(
	request database.QueryRequest,
) response.BaseResponse[database.ExplainPlan] {
	request.Query = strings.TrimSpace(request.Query)
	statements, err := database.SplitSQLStatements(request.Query)
	if err != nil || len(statements) != 1 {
		detail := "Explain accepts exactly one SQL statement."
		if err != nil {
			detail = err.Error()
		}
		return serviceErrorWithCode[database.ExplainPlan](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Cannot explain query batch",
			detail,
			"Select one statement in the editor and try again.",
		)
	}
	keywords := database.LeadingSQLKeywords(statements[0], 2)
	if len(keywords) > 0 {
		switch keywords[0] {
		case "EXPLAIN", "ANALYZE", "VACUUM", "PRAGMA", "ATTACH", "DETACH":
			return serviceErrorWithCode[database.ExplainPlan](
				http.StatusBadRequest,
				errorCodeInvalidRequest,
				"Unsafe explain input",
				fmt.Sprintf("%s cannot be wrapped in a safe visual explain.", keywords[0]),
				"Select the underlying SELECT, INSERT, UPDATE, or DELETE statement instead.",
			)
		}
	}

	driver, release, err := s.driverFor(request.ConnectionID)
	if err != nil {
		return serviceError[database.ExplainPlan](err.Error())
	}
	defer release()
	if !driver.Capabilities().ExplainPlans {
		return serviceErrorWithCode[database.ExplainPlan](
			http.StatusNotImplemented,
			errorCodeObjectMetadataUnsupported,
			"Explain plans are unavailable",
			"The active database driver does not support visual explain plans.",
			"Run the engine-specific EXPLAIN statement manually in a query tab.",
		)
	}
	explainDriver, ok := driver.(database.ExplainPlanDriver)
	if !ok {
		return serviceErrorWithCode[database.ExplainPlan](
			http.StatusNotImplemented,
			errorCodeObjectMetadataUnsupported,
			"Explain plans are unavailable",
			"The active driver advertises explain support but has no explain implementation.",
			"Update the driver before using the visual explain viewer.",
		)
	}
	boundQuery, args, err := database.BindQueryVariables(
		statements[0],
		driver,
		request.Variables,
	)
	if err != nil {
		return serviceErrorWithCode[database.ExplainPlan](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Query variables are incomplete",
			err.Error(),
			"Provide a value for every {{variable}} before explaining the query.",
		)
	}
	// Explain drivers currently use database/sql directly. Binding is rendered
	// through safe driver placeholders; the optional interface below preserves
	// parameter values without interpolation.
	if len(args) > 0 {
		if parameterized, supported := explainDriver.(interface {
			ExplainQueryWithArgs(
				context.Context,
				string,
				[]interface{},
			) (database.ExplainPlan, error)
		}); supported {
			parent := s.ctx
			if parent == nil {
				parent = context.Background()
			}
			ctx, cancel := context.WithTimeout(parent, explainQueryTimeout)
			defer cancel()
			plan, explainErr := parameterized.ExplainQueryWithArgs(ctx, boundQuery, args)
			if explainErr != nil {
				return queryFailure[database.ExplainPlan](explainErr, false)
			}
			return response.BaseResponse[database.ExplainPlan]{Data: plan}
		}
		return serviceErrorWithCode[database.ExplainPlan](
			http.StatusNotImplemented,
			errorCodeInvalidRequest,
			"Parameterized explain is unavailable",
			"The active driver cannot explain a statement with variables.",
			"Temporarily replace the variables with safe literals or update the driver.",
		)
	}

	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, explainQueryTimeout)
	defer cancel()
	plan, err := explainDriver.ExplainQuery(ctx, boundQuery)
	if err != nil {
		return queryFailure[database.ExplainPlan](err, false)
	}
	return response.BaseResponse[database.ExplainPlan]{Data: plan}
}

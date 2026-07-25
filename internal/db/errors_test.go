package db

import (
	"context"
	"testing"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
	oraclenetwork "github.com/sijms/go-ora/v2/network"
)

func testConnectionAttempt() *connectionAttempt {
	return &connectionAttempt{
		ctx:     context.Background(),
		timeout: 15 * time.Second,
	}
}

func TestOracleErrorsReceiveActionableCodes(t *testing.T) {
	authentication := connectionFailure[bool](
		testConnectionAttempt(),
		&oraclenetwork.OracleError{
			ErrCode: 1017,
			ErrMsg:  "ORA-01017: invalid username/password",
		},
	)
	if len(authentication.Errors) != 1 ||
		authentication.Errors[0].Code != errorCodeAuthenticationFailed {
		t.Fatalf("authentication response = %+v", authentication)
	}
	service := connectionFailure[bool](
		testConnectionAttempt(),
		&oraclenetwork.OracleError{
			ErrCode: 12514,
			ErrMsg:  "ORA-12514: listener does not know service",
		},
	)
	if len(service.Errors) != 1 ||
		service.Errors[0].Code != errorCodeDatabaseNotFound {
		t.Fatalf("service response = %+v", service)
	}
	constraint := queryFailure[bool](
		&oraclenetwork.OracleError{
			ErrCode: 1,
			ErrMsg:  "ORA-00001: unique constraint violated",
		},
		false,
	)
	if len(constraint.Errors) != 1 ||
		constraint.Errors[0].Code != errorCodeQueryConstraint {
		t.Fatalf("constraint response = %+v", constraint)
	}
}

func TestSQLServerErrorsReceiveActionableCodes(t *testing.T) {
	authentication := connectionFailure[bool](
		testConnectionAttempt(),
		mssql.Error{Number: 18456, Message: "Login failed"},
	)
	if len(authentication.Errors) != 1 ||
		authentication.Errors[0].Code != errorCodeAuthenticationFailed {
		t.Fatalf("authentication response = %+v", authentication)
	}
	syntax := queryFailure[bool](
		mssql.Error{
			Number:  207,
			Message: "Invalid column name",
			LineNo:  3,
		},
		false,
	)
	if len(syntax.Errors) != 1 ||
		syntax.Errors[0].Code != errorCodeQuerySyntax {
		t.Fatalf("syntax response = %+v", syntax)
	}
	constraint := queryFailure[bool](
		mssql.Error{Number: 547, Message: "Constraint conflict"},
		false,
	)
	if len(constraint.Errors) != 1 ||
		constraint.Errors[0].Code != errorCodeQueryConstraint {
		t.Fatalf("constraint response = %+v", constraint)
	}
}

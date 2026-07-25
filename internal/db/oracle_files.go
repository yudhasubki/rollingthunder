package db

import (
	"path/filepath"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/oracle"
	"rollingthunder/pkg/response"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var oracleTNSFileFilters = []wailsruntime.FileFilter{
	{
		DisplayName: "Oracle Net service names (*.ora)",
		Pattern:     "*.ora",
	},
	{
		DisplayName: "All files",
		Pattern:     "*",
	},
}

func (s *Service) ChooseOracleTNSFile() response.BaseResponse[database.OracleTNSSelection] {
	if s.ctx == nil {
		return serviceErrorWithCode[database.OracleTNSSelection](
			500,
			errorCodeDatabaseOperationFailed,
			"Application is not ready",
			"The native file picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	path, err := s.oracleTNSOpenDialog(s.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Choose tnsnames.ora",
		Filters:              oracleTNSFileFilters,
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return serviceErrorWithCode[database.OracleTNSSelection](
			500,
			errorCodeDatabaseOperationFailed,
			"Could not choose tnsnames.ora",
			err.Error(),
			"Check file permissions and try the native file picker again.",
		)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return response.BaseResponse[database.OracleTNSSelection]{
			Data: database.OracleTNSSelection{Aliases: []string{}},
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return serviceErrorWithCode[database.OracleTNSSelection](
			400,
			errorCodeInvalidRequest,
			"Invalid tnsnames.ora path",
			err.Error(),
			"Choose another Oracle Net configuration file.",
		)
	}
	absolute = filepath.Clean(absolute)
	aliases, err := oracle.ReadTNSAliases(absolute)
	if err != nil {
		return serviceErrorWithCode[database.OracleTNSSelection](
			400,
			errorCodeInvalidRequest,
			"Could not read tnsnames.ora",
			err.Error(),
			"Choose a valid tnsnames.ora file containing at least one connect alias.",
		)
	}
	return response.BaseResponse[database.OracleTNSSelection]{
		Data: database.OracleTNSSelection{
			Path:    absolute,
			Aliases: aliases,
		},
	}
}

func (s *Service) ChooseOracleWalletDirectory() response.BaseResponse[database.OracleWalletSelection] {
	if s.ctx == nil {
		return serviceErrorWithCode[database.OracleWalletSelection](
			500,
			errorCodeDatabaseOperationFailed,
			"Application is not ready",
			"The native directory picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	path, err := s.oracleWalletDialog(s.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Choose Oracle Wallet directory",
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return serviceErrorWithCode[database.OracleWalletSelection](
			500,
			errorCodeDatabaseOperationFailed,
			"Could not choose Oracle Wallet",
			err.Error(),
			"Check directory permissions and try the native picker again.",
		)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return response.BaseResponse[database.OracleWalletSelection]{}
	}
	info, err := oracle.InspectWalletDirectory(path)
	if err != nil {
		return serviceErrorWithCode[database.OracleWalletSelection](
			400,
			errorCodeInvalidRequest,
			"Invalid Oracle Wallet directory",
			err.Error(),
			"Choose a directory containing ewallet.p12 or cwallet.sso.",
		)
	}
	return response.BaseResponse[database.OracleWalletSelection]{
		Data: database.OracleWalletSelection{
			Path:             info.Path,
			HasAutoLogin:     info.HasAutoLogin,
			PasswordRequired: info.HasEWallet && !info.HasAutoLogin,
		},
	}
}

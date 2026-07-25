package oracle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WalletDirectoryInfo struct {
	Path         string
	HasEWallet   bool
	HasAutoLogin bool
}

func inspectWalletFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s must be a regular file", filepath.Base(path))
	}
	return true, nil
}

// InspectWalletDirectory validates a local Oracle Wallet selection. It checks
// only the expected wallet files; passwords remain in the credential store.
func InspectWalletDirectory(path string) (WalletDirectoryInfo, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return WalletDirectoryInfo{}, fmt.Errorf("Oracle Wallet path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"resolve Oracle Wallet path: %w",
			err,
		)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"inspect Oracle Wallet directory: %w",
			err,
		)
	}
	if !info.IsDir() {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"Oracle Wallet path must be a directory",
		)
	}
	hasEWallet, err := inspectWalletFile(filepath.Join(absolute, "ewallet.p12"))
	if err != nil {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"inspect Oracle Wallet password file: %w",
			err,
		)
	}
	hasAutoLogin, err := inspectWalletFile(filepath.Join(absolute, "cwallet.sso"))
	if err != nil {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"inspect Oracle Wallet auto-login file: %w",
			err,
		)
	}
	if !hasEWallet && !hasAutoLogin {
		return WalletDirectoryInfo{}, fmt.Errorf(
			"Oracle Wallet directory must contain ewallet.p12 or cwallet.sso",
		)
	}
	return WalletDirectoryInfo{
		Path:         absolute,
		HasEWallet:   hasEWallet,
		HasAutoLogin: hasAutoLogin,
	}, nil
}

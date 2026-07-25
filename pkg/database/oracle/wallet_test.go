package oracle

import (
	"os"
	"path/filepath"
	"testing"
)

func walletFixture(t *testing.T, files ...string) string {
	t.Helper()
	path := t.TempDir()
	for _, file := range files {
		if err := os.WriteFile(
			filepath.Join(path, file),
			[]byte("wallet"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestInspectWalletDirectoryRecognizesSupportedWalletFiles(t *testing.T) {
	autoLogin := walletFixture(t, "cwallet.sso")
	info, err := InspectWalletDirectory(autoLogin)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != autoLogin || info.HasEWallet || !info.HasAutoLogin {
		t.Fatalf("auto-login Wallet info = %+v", info)
	}

	passwordProtected := walletFixture(t, "ewallet.p12")
	info, err = InspectWalletDirectory(passwordProtected)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != passwordProtected || !info.HasEWallet || info.HasAutoLogin {
		t.Fatalf("password-protected Wallet info = %+v", info)
	}
}

func TestInspectWalletDirectoryRejectsMissingOrNonRegularWallet(t *testing.T) {
	if _, err := InspectWalletDirectory(t.TempDir()); err == nil {
		t.Fatal("empty Wallet directory was accepted")
	}
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "cwallet.sso"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectWalletDirectory(path); err == nil {
		t.Fatal("directory masquerading as cwallet.sso was accepted")
	}
}

func TestOracleWalletDirectoryEnforcesPasswordAndCertificateRules(t *testing.T) {
	autoLogin := walletFixture(t, "cwallet.sso")
	if path, err := oracleWalletDirectory(Config{
		WalletPath: autoLogin,
	}); err != nil || path != autoLogin {
		t.Fatalf("auto-login Wallet = %q, %v", path, err)
	}
	if _, err := oracleWalletDirectory(Config{
		WalletPath:     autoLogin,
		WalletPassword: "unexpected",
	}); err == nil {
		t.Fatal("password without ewallet.p12 was accepted")
	}

	passwordProtected := walletFixture(t, "ewallet.p12")
	if _, err := oracleWalletDirectory(Config{
		WalletPath: passwordProtected,
	}); err == nil {
		t.Fatal("ewallet.p12 without password was accepted")
	}
	if path, err := oracleWalletDirectory(Config{
		WalletPath:     passwordProtected,
		WalletPassword: "secret",
	}); err != nil || path != passwordProtected {
		t.Fatalf("password-protected Wallet = %q, %v", path, err)
	}
	if _, err := oracleWalletDirectory(Config{
		WalletPath:  autoLogin,
		SSLRootCert: "/separate/root.pem",
	}); err == nil {
		t.Fatal("Wallet with separate CA certificate was accepted")
	}
	if _, err := oracleWalletDirectory(Config{
		WalletPath: autoLogin,
		SSLMode:    "verify-ca",
	}); err == nil {
		t.Fatal("Wallet with misleading CA-only verification was accepted")
	}
}

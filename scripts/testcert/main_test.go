package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateFixturesCreatesTrustedServerAndClientCertificates(
	t *testing.T,
) {
	t.Parallel()
	outputDirectory := filepath.Join(t.TempDir(), "fixtures")
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	if err := generateFixtures(fixtureOptions{
		outputDirectory:  outputDirectory,
		clientCommonName: "rolling_tls",
		serverCommonName: "sqlserver.test",
		now:              now,
	}); err != nil {
		t.Fatal(err)
	}

	root := readCertificate(t, filepath.Join(outputDirectory, "ca-cert.pem"))
	server := readCertificate(
		t,
		filepath.Join(outputDirectory, "server-cert.pem"),
	)
	client := readCertificate(
		t,
		filepath.Join(outputDirectory, "client-cert.pem"),
	)
	wrongRoot := readCertificate(
		t,
		filepath.Join(outputDirectory, "wrong-ca-cert.pem"),
	)

	roots := x509.NewCertPool()
	roots.AddCert(root)
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:   roots,
		DNSName: "localhost",
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		CurrentTime: now,
	}); err != nil {
		t.Fatalf("verify generated server certificate: %v", err)
	}
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:   roots,
		DNSName: "sqlserver.test",
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		CurrentTime: now,
	}); err != nil {
		t.Fatalf("verify generated server FQDN certificate: %v", err)
	}
	if server.Subject.CommonName != "sqlserver.test" {
		t.Fatalf(
			"server common name = %q, want sqlserver.test",
			server.Subject.CommonName,
		)
	}
	if !server.BasicConstraintsValid || server.IsCA {
		t.Fatalf(
			"server basic constraints = valid:%t ca:%t, want valid end-entity certificate",
			server.BasicConstraintsValid,
			server.IsCA,
		)
	}
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:   roots,
		DNSName: "127.0.0.1",
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		CurrentTime: now,
	}); err != nil {
		t.Fatalf("verify generated server IP certificate: %v", err)
	}
	if _, err := client.Verify(x509.VerifyOptions{
		Roots: roots,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
		CurrentTime: now,
	}); err != nil {
		t.Fatalf("verify generated client certificate: %v", err)
	}
	if client.Subject.CommonName != "rolling_tls" {
		t.Fatalf(
			"client common name = %q, want rolling_tls",
			client.Subject.CommonName,
		)
	}

	wrongRoots := x509.NewCertPool()
	wrongRoots.AddCert(wrongRoot)
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:   wrongRoots,
		DNSName: "localhost",
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		CurrentTime: now,
	}); err == nil {
		t.Fatal("server certificate unexpectedly trusted by the unrelated CA")
	}

	assertFileMode(
		t,
		filepath.Join(outputDirectory, "server-key.pem"),
		0o600,
	)
	assertFileMode(
		t,
		filepath.Join(outputDirectory, "client-key.pem"),
		0o600,
	)
}

func TestGenerateFixturesRejectsUnsafeClientCommonName(t *testing.T) {
	t.Parallel()
	err := generateFixtures(fixtureOptions{
		outputDirectory:  t.TempDir(),
		clientCommonName: "rolling/client",
		now:              time.Now(),
	})
	if err == nil {
		t.Fatal("unsafe client common name was accepted")
	}
}

func TestGenerateFixturesRejectsUnsafeServerCommonName(t *testing.T) {
	t.Parallel()
	err := generateFixtures(fixtureOptions{
		outputDirectory:  t.TempDir(),
		serverCommonName: "sqlserver/test",
		now:              time.Now(),
	})
	if err == nil {
		t.Fatal("unsafe server common name was accepted")
	}
}

func TestGenerateFixturesCanOmitClientCertificate(t *testing.T) {
	t.Parallel()
	outputDirectory := filepath.Join(t.TempDir(), "fixtures")
	if err := os.MkdirAll(outputDirectory, outputDirectoryMode); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"client-cert.pem", "client-key.pem"} {
		if err := os.WriteFile(
			filepath.Join(outputDirectory, name),
			[]byte("stale"),
			privateFileMode,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := generateFixtures(fixtureOptions{
		outputDirectory: outputDirectory,
		now:             time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ca-cert.pem",
		"server-cert.pem",
		"server-key.pem",
		"wrong-ca-cert.pem",
	} {
		if _, err := os.Stat(filepath.Join(outputDirectory, name)); err != nil {
			t.Fatalf("expected server fixture %s: %v", name, err)
		}
	}
	for _, name := range []string{"client-cert.pem", "client-key.pem"} {
		if _, err := os.Stat(filepath.Join(outputDirectory, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected client fixture %s: %v", name, err)
		}
	}
}

func readCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("%s does not contain a certificate PEM block", path)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

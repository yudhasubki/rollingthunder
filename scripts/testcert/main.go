// Command testcert generates short-lived TLS fixtures for disposable database
// conformance environments. It is intentionally built from the Go standard
// library so CI does not depend on an external certificate generator.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	certificateValidity = 7 * 24 * time.Hour
	rsaKeyBits          = 2048
	privateFileMode     = 0o600
	publicFileMode      = 0o644
	outputDirectoryMode = 0o700
)

var commonNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type certificateAuthority struct {
	certificate *x509.Certificate
	key         *rsa.PrivateKey
}

type fixtureOptions struct {
	outputDirectory  string
	clientCommonName string
	now              time.Time
}

type fixtureFile struct {
	name  string
	block *pem.Block
	mode  os.FileMode
}

func main() {
	outputDirectory := flag.String(
		"output",
		"",
		"directory that receives the generated PEM fixtures",
	)
	clientCommonName := flag.String(
		"client-common-name",
		"",
		"optional common name embedded in a generated client certificate",
	)
	flag.Parse()

	if err := generateFixtures(fixtureOptions{
		outputDirectory:  *outputDirectory,
		clientCommonName: *clientCommonName,
		now:              time.Now().UTC(),
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateFixtures(options fixtureOptions) error {
	if options.outputDirectory == "" {
		return errors.New("output directory is required")
	}
	if options.clientCommonName != "" &&
		!commonNamePattern.MatchString(options.clientCommonName) {
		return errors.New("client common name contains unsupported characters")
	}
	if options.now.IsZero() {
		options.now = time.Now().UTC()
	}
	if err := os.MkdirAll(options.outputDirectory, outputDirectoryMode); err != nil {
		return fmt.Errorf("create TLS fixture directory: %w", err)
	}
	if err := os.Chmod(options.outputDirectory, outputDirectoryMode); err != nil {
		return fmt.Errorf("restrict TLS fixture directory: %w", err)
	}

	root, rootPEM, err := newCertificateAuthority(
		"Rolling Thunder CI Root CA",
		options.now,
	)
	if err != nil {
		return err
	}
	_, wrongRootPEM, err := newCertificateAuthority(
		"Rolling Thunder CI Untrusted CA",
		options.now,
	)
	if err != nil {
		return err
	}

	serverCertificate, serverKey, err := newLeafCertificate(
		root,
		leafOptions{
			commonName: "localhost",
			dnsNames:   []string{"localhost"},
			ipAddresses: []net.IP{
				net.ParseIP("127.0.0.1"),
			},
			extendedKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
			now: options.now,
		},
	)
	if err != nil {
		return err
	}
	files := []fixtureFile{
		{
			name:  "ca-cert.pem",
			block: &pem.Block{Type: "CERTIFICATE", Bytes: rootPEM},
			mode:  publicFileMode,
		},
		{
			name: "server-cert.pem",
			block: &pem.Block{
				Type:  "CERTIFICATE",
				Bytes: serverCertificate,
			},
			mode: publicFileMode,
		},
		{
			name: "server-key.pem",
			block: &pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(serverKey),
			},
			mode: privateFileMode,
		},
		{
			name: "wrong-ca-cert.pem",
			block: &pem.Block{
				Type:  "CERTIFICATE",
				Bytes: wrongRootPEM,
			},
			mode: publicFileMode,
		},
	}
	if options.clientCommonName != "" {
		clientCertificate, clientKey, err := newLeafCertificate(
			root,
			leafOptions{
				commonName: options.clientCommonName,
				extendedKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
				now: options.now,
			},
		)
		if err != nil {
			return err
		}
		files = append(files,
			fixtureFile{
				name: "client-cert.pem",
				block: &pem.Block{
					Type:  "CERTIFICATE",
					Bytes: clientCertificate,
				},
				mode: publicFileMode,
			},
			fixtureFile{
				name: "client-key.pem",
				block: &pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
				},
				mode: privateFileMode,
			},
		)
	}
	for _, file := range files {
		if err := writePEMAtomically(
			filepath.Join(options.outputDirectory, file.name),
			file.block,
			file.mode,
		); err != nil {
			return err
		}
	}
	if options.clientCommonName == "" {
		for _, name := range []string{"client-cert.pem", "client-key.pem"} {
			if err := os.Remove(
				filepath.Join(options.outputDirectory, name),
			); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stale TLS fixture %s: %w", name, err)
			}
		}
	}
	return nil
}

func newCertificateAuthority(
	commonName string,
	now time.Time,
) (certificateAuthority, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return certificateAuthority{}, nil, fmt.Errorf(
			"generate CA private key: %w",
			err,
		)
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return certificateAuthority{}, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(certificateValidity),
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign |
			x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&key.PublicKey,
		key,
	)
	if err != nil {
		return certificateAuthority{}, nil, fmt.Errorf(
			"create CA certificate: %w",
			err,
		)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return certificateAuthority{}, nil, fmt.Errorf(
			"parse generated CA certificate: %w",
			err,
		)
	}
	return certificateAuthority{certificate: certificate, key: key}, der, nil
}

type leafOptions struct {
	commonName       string
	dnsNames         []string
	ipAddresses      []net.IP
	extendedKeyUsage []x509.ExtKeyUsage
	now              time.Time
}

func newLeafCertificate(
	root certificateAuthority,
	options leafOptions,
) ([]byte, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, nil, fmt.Errorf("generate leaf private key: %w", err)
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: options.commonName},
		NotBefore:    options.now.Add(-time.Minute),
		NotAfter:     options.now.Add(certificateValidity),
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: options.extendedKeyUsage,
		DNSNames:    options.dnsNames,
		IPAddresses: options.ipAddresses,
	}
	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		root.certificate,
		&key.PublicKey,
		root.key,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create leaf certificate: %w", err)
	}
	return der, key, nil
}

func randomSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial number: %w", err)
	}
	if serial.Sign() == 0 {
		return big.NewInt(1), nil
	}
	return serial, nil
}

func writePEMAtomically(
	path string,
	block *pem.Block,
	mode os.FileMode,
) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".rolling-thunder-cert-*")
	if err != nil {
		return fmt.Errorf("create temporary certificate file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("restrict temporary certificate file: %w", err)
	}
	if err := pem.Encode(temporary, block); err != nil {
		return fmt.Errorf("encode certificate fixture: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync certificate fixture: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close certificate fixture: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace certificate fixture: %w", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set certificate fixture mode: %w", err)
	}
	return nil
}

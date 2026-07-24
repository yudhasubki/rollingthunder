package main

import "testing"

func TestProductVersionFromWailsConfig(t *testing.T) {
	version, err := productVersionFromWailsConfig([]byte(
		`{"info":{"productVersion":" 1.4.2 "}}`,
	))
	if err != nil {
		t.Fatalf("productVersionFromWailsConfig() error = %v", err)
	}
	if version != "1.4.2" {
		t.Fatalf("version = %q", version)
	}
}

func TestProductVersionFromWailsConfigRejectsMissingVersion(t *testing.T) {
	if _, err := productVersionFromWailsConfig([]byte(`{"info":{}}`)); err == nil {
		t.Fatal("missing productVersion was accepted")
	}
}

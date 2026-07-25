package db

import (
	"errors"

	"rollingthunder/pkg/application"

	"github.com/zalando/go-keyring"
)

var ErrCredentialNotFound = errors.New("credential not found")

type CredentialStore interface {
	Set(profileID string, password string) error
	Get(profileID string) (string, error)
	Delete(profileID string) error
}

type operatingSystemCredentialStore struct{}

func newOperatingSystemCredentialStore() CredentialStore {
	return operatingSystemCredentialStore{}
}

func (operatingSystemCredentialStore) Set(profileID string, password string) error {
	return keyring.Set(application.CredentialServiceName, profileID, password)
}

func (operatingSystemCredentialStore) Get(profileID string) (string, error) {
	password, err := keyring.Get(application.CredentialServiceName, profileID)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrCredentialNotFound
	}
	return password, err
}

func (operatingSystemCredentialStore) Delete(profileID string) error {
	err := keyring.Delete(application.CredentialServiceName, profileID)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrCredentialNotFound
	}
	return err
}

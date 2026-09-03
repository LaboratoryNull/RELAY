package main

import "github.com/zalando/go-keyring"

const credentialService = "Relay SSH Manager"

// Passwords live in the operating system credential vault (Secret Service on
// Linux, Keychain on macOS, Credential Manager on Windows) and never enter the
// hosts JSON or get returned to JavaScript.
func (a *App) setCredential(hostID, password string) error {
	return keyring.Set(credentialService, hostID, password)
}

func (a *App) getCredential(hostID string) string {
	password, err := keyring.Get(credentialService, hostID)
	if err != nil {
		return ""
	}
	return password
}

func (a *App) deleteCredential(hostID string) error {
	err := keyring.Delete(credentialService, hostID)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

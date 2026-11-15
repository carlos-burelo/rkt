//go:build !windows

package cli

import (
	"errors"
	"log"
)

func platformRequireAdmin() error { return errors.New("solo Windows") }

func scQueryService(svc string) (string, error) { return "", errors.New("solo Windows") }

func scQueryState(svc string) (int, error) { return 0, errors.New("solo Windows") }

func setServiceBinpath(svc, exe, cfg string) { log.Fatal("rkt upgrade: solo Windows") }

func restartService(svc string) { log.Fatal("rkt upgrade: solo Windows") }

func bestEffortRecovery(svc string) {}

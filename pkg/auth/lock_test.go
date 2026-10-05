/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const lockTestServer = "https://reana.example.org"

func lockTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Path: filepath.Join(t.TempDir(), "reana-client.json")}
}

func refreshLockTestPath(t *testing.T, store *Store) string {
	t.Helper()
	path, err := store.refreshLockPath(lockTestServer)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// plantSymlink puts an attacker-style symlink at lockPath and returns its
// target, a world-readable file whose mode must stay untouched.
func plantSymlink(t *testing.T, lockPath string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("victim"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}
	return target
}

func assertLockRefused(t *testing.T, err error, lockPath string) {
	t.Helper()
	if err == nil ||
		!strings.Contains(err.Error(), "not a regular file") ||
		!strings.Contains(err.Error(), lockPath) {
		t.Fatalf("lock error = %v, want refusal naming %s", err, lockPath)
	}
}

func assertTargetUntouched(t *testing.T, target string) {
	t.Helper()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target mode = %o, want 644", info.Mode().Perm())
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "victim" {
		t.Fatalf("symlink target contents = %q, %v", contents, err)
	}
}

func TestStoreLockRefusesSymlink(t *testing.T) {
	store := lockTestStore(t)
	lockPath := store.Path + ".lock"
	target := plantSymlink(t, lockPath)

	_, err := store.Put(lockTestServer, Credentials{AccessToken: "a"}, true)
	assertLockRefused(t, err, lockPath)
	_, err = store.Get(lockTestServer)
	assertLockRefused(t, err, lockPath)
	_, err = store.ActiveServer()
	assertLockRefused(t, err, lockPath)

	assertTargetUntouched(t, target)
	if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential file was written despite the refusal: %v", err)
	}
}

func TestStoreLockRefusesDanglingSymlink(t *testing.T) {
	store := lockTestStore(t)
	lockPath := store.Path + ".lock"
	target := filepath.Join(t.TempDir(), "missing")
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}

	_, err := store.Get(lockTestServer)
	assertLockRefused(t, err, lockPath)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target was created: %v", err)
	}
}

func TestStoreLockRefusesNonRegularFile(t *testing.T) {
	store := lockTestStore(t)
	lockPath := store.Path + ".lock"
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := store.Get(lockTestServer)
	assertLockRefused(t, err, lockPath)

	fifoPath := refreshLockTestPath(t, store)
	if err := unix.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := store.tryRefreshLock(lockTestServer)
	if lock != nil {
		releaseLock(lock)
	}
	assertLockRefused(t, err, fifoPath)
}

func TestRefreshLockRefusesSymlink(t *testing.T) {
	store := lockTestStore(t)
	lockPath := refreshLockTestPath(t, store)
	target := plantSymlink(t, lockPath)

	lock, err := store.tryRefreshLock(lockTestServer)
	if lock != nil {
		releaseLock(lock)
	}
	assertLockRefused(t, err, lockPath)
	_, err = store.waitRefreshLock(lockTestServer, lockPollInterval)
	assertLockRefused(t, err, lockPath)
	assertTargetUntouched(t, target)
}

func TestRefreshRefusesSymlinkedRefreshLock(t *testing.T) {
	requested := false
	manager := testManager(t, func(*http.Request) (*http.Response, error) {
		requested = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	credentials, err := manager.Store.Put(
		lockTestServer,
		expiredCredentials(manager.Now()),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := refreshLockTestPath(t, manager.Store)
	target := plantSymlink(t, lockPath)

	_, err = manager.Refresh(context.Background(), lockTestServer, credentials)
	assertLockRefused(t, err, lockPath)
	assertTargetUntouched(t, target)
	if requested {
		t.Fatal("refresh token was sent despite the refused lock")
	}
}

func TestOrdinaryLockFilesAreCreatedPrivate(t *testing.T) {
	store := lockTestStore(t)
	if _, err := store.Put(
		lockTestServer,
		Credentials{AccessToken: "a"},
		true,
	); err != nil {
		t.Fatal(err)
	}
	lock, err := store.tryRefreshLock(lockTestServer)
	if err != nil || lock == nil {
		t.Fatalf("refresh lock = %v, %v", lock, err)
	}
	second, err := store.tryRefreshLock(lockTestServer)
	if err != nil || second != nil {
		t.Fatalf("held refresh lock was acquired again: %v, %v", second, err)
	}
	releaseLock(lock)

	for _, path := range []string{
		store.Path + ".lock",
		refreshLockTestPath(t, store),
	} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v, want regular 0600", path, info.Mode())
		}
	}
}

func TestExistingLockFileIsTightenedAndReused(t *testing.T) {
	store := lockTestStore(t)
	lockPath := store.Path + ".lock"
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(lockTestServer); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode = %o, want 600", info.Mode().Perm())
	}
}

//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package specbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openRegularFileBeneath opens every path component relative to an already
// pinned directory descriptor. A rename or symlink swap after scope discovery
// therefore cannot redirect the archive read outside its source directory.
func openRegularFileBeneath(
	baseDir, relativePath, field string,
) (*os.File, error) {
	relativePath, err := normalizeDeclaredPath(relativePath, field)
	if err != nil {
		return nil, err
	}
	absoluteBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	relativeComponents := strings.Split(relativePath, "/")
	directoryFlags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	// The caller-provided base is trusted and may itself be a symlink (for
	// example /var on macOS). Pin it directly, then refuse links beneath it.
	descriptor, err := unix.Open(
		absoluteBase,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("could not securely open %s: %w", field, err)
	}
	defer func() {
		if descriptor >= 0 {
			_ = unix.Close(descriptor)
		}
	}()

	for _, component := range relativeComponents[:len(relativeComponents)-1] {
		nextDescriptor, openErr := unix.Openat(
			descriptor,
			component,
			directoryFlags,
			0,
		)
		if openErr != nil {
			return nil, fmt.Errorf(
				"could not securely open declared path in %s: %s (%w)",
				field,
				relativePath,
				openErr,
			)
		}
		_ = unix.Close(descriptor)
		descriptor = nextDescriptor
	}
	fileFlags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	fileDescriptor, err := unix.Openat(
		descriptor,
		relativeComponents[len(relativeComponents)-1],
		fileFlags,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"could not securely open declared path in %s: %s (%w)",
			field,
			relativePath,
			err,
		)
	}
	var metadata unix.Stat_t
	if err := unix.Fstat(fileDescriptor, &metadata); err != nil {
		_ = unix.Close(fileDescriptor)
		return nil, err
	}
	if metadata.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fileDescriptor)
		return nil, fmt.Errorf(
			"declared path is not a regular file in %s: %s",
			field,
			relativePath,
		)
	}
	return os.NewFile(uintptr(fileDescriptor), relativePath), nil
}

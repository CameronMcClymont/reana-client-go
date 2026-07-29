//go:build !(aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris)

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
)

// openRegularFileBeneath provides the strongest available fallback on systems
// without descriptor-relative O_NOFOLLOW traversal.
func openRegularFileBeneath(
	baseDir, relativePath, field string,
) (*os.File, error) {
	relativePath, err := normalizeDeclaredPath(relativePath, field)
	if err != nil {
		return nil, err
	}
	sourcePath, err := ensureRegularPath(baseDir, relativePath, field)
	if err != nil {
		return nil, err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	opened, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	current, err := os.Lstat(sourcePath)
	if err != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) {
		_ = source.Close()
		return nil, fmt.Errorf(
			"declared path changed while opening in %s: %s",
			field,
			relativePath,
		)
	}
	return source, nil
}

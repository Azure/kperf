// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package localstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrAlreadyExists returns the content exists.
//
// TODO(weifu): move it into common pkg.
var ErrAlreadyExists = errors.New("already exists")

// writer implements Writer interface.
type writer struct {
	s *Store

	name string
	f    *os.File
}

// Write writes data into underlying file.
func (w *writer) Write(data []byte) (int, error) {
	return w.f.Write(data)
}

// Close closes file and remove it.
func (w *writer) Close() error {
	w.f.Close()
	if err := os.Remove(w.name); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Commit commits data as file named by ref.
func (w *writer) Commit(ref string) error {
	w.s.Lock()
	defer w.s.Unlock()

	defer w.Close()

	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("failed to fsync: %w", err)
	}

	target := filepath.Join(w.s.dataDir, ref)
	_, err := os.Stat(target)
	if err == nil {
		return fmt.Errorf("ref %s already exists: %w", ref, ErrAlreadyExists)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to ensure if ref %s exists: %w", ref, err)
	}
	return os.Rename(w.name, target)
}

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package localstore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Store is a filesystem-like key/value storage.
//
// Each key/value has committed and ingesting status. When OpenWriter returns
// ingestion transcation, the Store opens rootDir/ingest/$random file to
// receive value data. Once all the data is written, the Commit(ref) moves the
// file into rootDir/data/ref.
type Store struct {
	sync.Mutex

	dataDir   string
	ingestDir string
}

// NewStore returns new instance of Store.
func NewStore(rootDir string) (*Store, error) {
	if !filepath.IsAbs(rootDir) {
		return nil, fmt.Errorf("%s is not absolute path", rootDir)
	}

	dataDir := filepath.Join(rootDir, "data")
	if err := os.MkdirAll(dataDir, 0600); err != nil {
		return nil, fmt.Errorf("failed to ensure data dir %s: %w", dataDir, err)
	}

	ingestDir := filepath.Join(rootDir, "ingest")
	if err := os.MkdirAll(ingestDir, 0600); err != nil {
		return nil, fmt.Errorf("failed to ensure ingest dir %s: %w", ingestDir, err)
	}

	return &Store{
		dataDir:   dataDir,
		ingestDir: ingestDir,
	}, nil
}

// OpenWriter is to initiate a writing operation, ingestion transcation. A
// single ingestion transcation is to open temporary file and allow caller to
// write data into the temporary file. Once all the data is written, the caller
// should call Commit to complete ingestion transcation.
func (s *Store) OpenWriter() (Writer, error) {
	f, err := os.CreateTemp(s.ingestDir, "ingest-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create ingest file: %w", err)
	}

	return &writer{
		s:    s,
		name: f.Name(),
		f:    f,
	}, nil
}

// OpenReader is to open committed content named by ref.
func (s *Store) OpenReader(ref string) (Reader, error) {
	s.Lock()
	defer s.Unlock()

	target := filepath.Join(s.dataDir, ref)

	stat, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure if ref %s exists: %w", ref, err)
	}

	size := stat.Size()
	f, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("failed to open ref %s: %w", ref, err)
	}

	return &sizeReadCloser{
		File: f,
		size: size,
	}, nil
}

// Delete is to delete committed content named by ref.
func (s *Store) Delete(ref string) error {
	s.Lock()
	defer s.Unlock()

	target := filepath.Join(s.dataDir, ref)
	_, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to ensure if ref %s exists: %w", ref, err)
	}
	return os.Remove(target)
}

// Writer handles writing of content into local store
type Writer interface {
	// Close closes the writer.
	//
	// If the writer has not been committed, this allows aborting.
	// Calling Close on a closed writer will not error.
	io.WriteCloser

	// Commit commits data as file named by ref.
	//
	// Commit always close Writer. If ref already exists, it will return
	// error.
	Commit(ref string) error
}

// Reader extends io.ReadCloser interface with io.ReaderAt and reporting of Size.
type Reader interface {
	io.ReaderAt
	io.ReadCloser
	Size() int64
}

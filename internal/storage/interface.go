/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"context"
	"io"
)

// Backend defines the interface for storage backends
type Backend interface {
	// Write writes data to the storage backend at the specified path
	Write(ctx context.Context, path string, reader io.Reader) error

	// Read reads data from the storage backend at the specified path
	Read(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete deletes the object at the specified path
	Delete(ctx context.Context, path string) error

	// Exists checks if an object exists at the specified path
	Exists(ctx context.Context, path string) (bool, error)

	// List lists objects with the specified prefix
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// GetSize returns the size of the object at the specified path
	GetSize(ctx context.Context, path string) (int64, error)

	// Close closes the storage backend and releases resources
	Close() error
}

// ObjectInfo contains information about a stored object
type ObjectInfo struct {
	// Path is the full path to the object
	Path string

	// Size is the size in bytes
	Size int64

	// LastModified is the last modification time as Unix timestamp
	LastModified int64

	// Checksum is the object checksum (MD5 or similar)
	Checksum string
}

// WriteCloser wraps io.WriteCloser with additional metadata
type WriteCloser interface {
	io.WriteCloser

	// Path returns the path where data is being written
	Path() string
}

// writeCloserImpl implements WriteCloser
type writeCloserImpl struct {
	io.WriteCloser
	path string
}

func (w *writeCloserImpl) Path() string {
	return w.path
}

// NewWriteCloser creates a new WriteCloser
func NewWriteCloser(w io.WriteCloser, path string) WriteCloser {
	return &writeCloserImpl{
		WriteCloser: w,
		path:        path,
	}
}

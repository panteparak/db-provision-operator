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
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Compressor provides compression and decompression functionality
type Compressor interface {
	// Compress returns a writer that compresses data written to it
	Compress(w io.Writer) (io.WriteCloser, error)

	// Decompress returns a reader that decompresses data read from it
	Decompress(r io.Reader) (io.ReadCloser, error)

	// Extension returns the file extension for the compression algorithm
	Extension() string
}

// NewCompressor creates a new compressor based on the configuration
func NewCompressor(config *dbopsv1alpha1.CompressionConfig) (Compressor, error) {
	if config == nil || !config.Enabled {
		return &noopCompressor{}, nil
	}

	switch config.Algorithm {
	case dbopsv1alpha1.CompressionGzip, "":
		return &gzipCompressor{level: int(config.Level)}, nil
	case dbopsv1alpha1.CompressionLZ4:
		return &lz4Compressor{level: int(config.Level)}, nil
	case dbopsv1alpha1.CompressionZstd:
		return &zstdCompressor{level: int(config.Level)}, nil
	case dbopsv1alpha1.CompressionNone:
		return &noopCompressor{}, nil
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", config.Algorithm)
	}
}

// noopCompressor provides no compression
type noopCompressor struct{}

func (c *noopCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return &nopWriteCloser{w}, nil
}

func (c *noopCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc, nil
	}
	return io.NopCloser(r), nil
}

func (c *noopCompressor) Extension() string {
	return ""
}

// nopWriteCloser wraps a writer with a no-op Close method
type nopWriteCloser struct {
	io.Writer
}

func (w *nopWriteCloser) Close() error {
	return nil
}

// gzipCompressor provides gzip compression
type gzipCompressor struct {
	level int
}

func (c *gzipCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	level := c.level
	if level == 0 {
		level = gzip.DefaultCompression
	}
	return gzip.NewWriterLevel(w, level)
}

func (c *gzipCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}

func (c *gzipCompressor) Extension() string {
	return ".gz"
}

// lz4Compressor provides LZ4 compression
type lz4Compressor struct {
	level int
}

func (c *lz4Compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	lz4Writer := lz4.NewWriter(w)
	if c.level > 0 {
		// LZ4 levels: 1-9 map to Fast compression, higher is slower but better ratio
		level := lz4.Fast
		switch {
		case c.level <= 3:
			level = lz4.Fast
		case c.level <= 6:
			level = lz4.Level5
		default:
			level = lz4.Level9
		}
		if err := lz4Writer.Apply(lz4.CompressionLevelOption(level)); err != nil {
			return nil, fmt.Errorf("failed to set lz4 compression level: %w", err)
		}
	}
	return lz4Writer, nil
}

func (c *lz4Compressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(lz4.NewReader(r)), nil
}

func (c *lz4Compressor) Extension() string {
	return ".lz4"
}

// zstdCompressor provides Zstandard compression
type zstdCompressor struct {
	level int
}

func (c *zstdCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	level := zstd.SpeedDefault
	if c.level > 0 {
		// Map 1-9 to zstd levels
		switch {
		case c.level <= 3:
			level = zstd.SpeedFastest
		case c.level <= 6:
			level = zstd.SpeedDefault
		default:
			level = zstd.SpeedBestCompression
		}
	}
	return zstd.NewWriter(w, zstd.WithEncoderLevel(level))
}

func (c *zstdCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &zstdReadCloser{decoder}, nil
}

func (c *zstdCompressor) Extension() string {
	return ".zst"
}

// zstdReadCloser wraps zstd.Decoder to implement io.ReadCloser
type zstdReadCloser struct {
	*zstd.Decoder
}

func (r *zstdReadCloser) Close() error {
	r.Decoder.Close()
	return nil
}

// CompressingWriter wraps a writer with compression
type CompressingWriter struct {
	underlying io.WriteCloser
	compressor io.WriteCloser
}

// NewCompressingWriter creates a writer that compresses data before writing
func NewCompressingWriter(w io.WriteCloser, compressor Compressor) (*CompressingWriter, error) {
	cw, err := compressor.Compress(w)
	if err != nil {
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}
	return &CompressingWriter{
		underlying: w,
		compressor: cw,
	}, nil
}

func (w *CompressingWriter) Write(p []byte) (n int, err error) {
	return w.compressor.Write(p)
}

func (w *CompressingWriter) Close() error {
	// Close compressor first to flush any buffered data
	if err := w.compressor.Close(); err != nil {
		w.underlying.Close()
		return err
	}
	return w.underlying.Close()
}

// DecompressingReader wraps a reader with decompression
type DecompressingReader struct {
	underlying   io.ReadCloser
	decompressor io.ReadCloser
}

// NewDecompressingReader creates a reader that decompresses data while reading
func NewDecompressingReader(r io.ReadCloser, compressor Compressor) (*DecompressingReader, error) {
	dr, err := compressor.Decompress(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}
	return &DecompressingReader{
		underlying:   r,
		decompressor: dr,
	}, nil
}

func (r *DecompressingReader) Read(p []byte) (n int, err error) {
	return r.decompressor.Read(p)
}

func (r *DecompressingReader) Close() error {
	// Close decompressor first
	if err := r.decompressor.Close(); err != nil {
		r.underlying.Close()
		return err
	}
	return r.underlying.Close()
}

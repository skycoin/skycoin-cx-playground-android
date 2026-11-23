// Package lz4codec holds the LZ4 block codec used to compress serialized CX
// programs. All of its state lives in a Codec value that the caller creates,
// initializes, passes and closes explicitly.
package lz4codec

import (
	"errors"
	"fmt"

	"github.com/pierrec/lz4/v4"
)

// MaxRatio bounds the output of an LZ4 block. A match length byte of 255 is
// the densest encoding, see https://github.com/lz4/lz4/blob/dev/doc/lz4_Block_format.md.
const MaxRatio = 255

var (
	// ErrNotInitialized is returned when a Codec is used before Init.
	ErrNotInitialized = errors.New("lz4codec: codec is not initialized, call Init first")
	// ErrInitialized is returned when Init is called twice.
	ErrInitialized = errors.New("lz4codec: codec is already initialized")
	// ErrClosed is returned when a Codec is used or initialized after Close.
	ErrClosed = errors.New("lz4codec: codec is closed")
	// ErrNilCodec is returned when a method is called on a nil *Codec.
	ErrNilCodec = errors.New("lz4codec: nil codec")
)

type codecState int

const (
	stateNew codecState = iota
	stateReady
	stateClosed
)

// Config sets the size limits of a Codec. Init allocates every buffer the
// codec uses from these limits, and no method allocates afterwards.
type Config struct {
	// MaxInput is the largest input Compress accepts, in bytes.
	MaxInput int
	// MaxOutput is the largest output Decompress produces, in bytes.
	MaxOutput int
}

// Codec compresses and decompresses LZ4 blocks. The zero value is not
// initialized. A Codec is not safe for concurrent use.
type Codec struct {
	cfg        Config
	state      codecState
	compressor *lz4.Compressor
	compBuf    []byte
	decompBuf  []byte
}

// Init checks cfg and allocates the compressor and both buffers.
func (c *Codec) Init(cfg Config) error {
	if c == nil {
		return ErrNilCodec
	}
	switch c.state {
	case stateReady:
		return ErrInitialized
	case stateClosed:
		return ErrClosed
	}
	if cfg.MaxInput <= 0 || cfg.MaxOutput <= 0 {
		return fmt.Errorf("lz4codec: MaxInput %d and MaxOutput %d must be positive", cfg.MaxInput, cfg.MaxOutput)
	}
	c.cfg = cfg
	c.compressor = new(lz4.Compressor)
	c.compBuf = make([]byte, lz4.CompressBlockBound(cfg.MaxInput))
	c.decompBuf = make([]byte, cfg.MaxOutput)
	c.state = stateReady
	return nil
}

// Close releases the buffers. A closed Codec cannot be used or initialized again.
func (c *Codec) Close() error {
	if err := c.ready(); err != nil {
		return err
	}
	c.compressor, c.compBuf, c.decompBuf = nil, nil, nil
	c.state = stateClosed
	return nil
}

// Config returns the limits set at Init.
func (c *Codec) Config() (Config, error) {
	if err := c.ready(); err != nil {
		return Config{}, err
	}
	return c.cfg, nil
}

func (c *Codec) ready() error {
	if c == nil {
		return ErrNilCodec
	}
	switch c.state {
	case stateNew:
		return ErrNotInitialized
	case stateClosed:
		return ErrClosed
	}
	return nil
}

// Compress returns src as an LZ4 block. Input that does not shrink is stored
// as literals, which is still a valid block. Empty input gives empty output,
// because lz4.UncompressBlock maps empty input to empty output. The result
// aliases the codec's buffer and is valid until the next call or Close.
func (c *Codec) Compress(src []byte) ([]byte, error) {
	if err := c.ready(); err != nil {
		return nil, err
	}
	if len(src) > c.cfg.MaxInput {
		return nil, fmt.Errorf("lz4codec: input of %d bytes exceeds MaxInput %d", len(src), c.cfg.MaxInput)
	}
	if len(src) == 0 {
		return c.compBuf[:0], nil
	}
	// Compressor clears its table on every call, so the output does not depend on earlier calls.
	n, err := c.compressor.CompressBlock(src, c.compBuf[:lz4.CompressBlockBound(len(src))])
	if err != nil {
		return nil, fmt.Errorf("lz4codec: compress: %w", err)
	}
	return c.compBuf[:n], nil
}

// Decompress returns the contents of the LZ4 block src. The original length
// is not stored, so the output may use up to MaxRatio times len(src), capped
// at MaxOutput. The result aliases the codec's buffer and is valid until the
// next call or Close.
func (c *Codec) Decompress(src []byte) ([]byte, error) {
	if err := c.ready(); err != nil {
		return nil, err
	}
	limit := min(c.cfg.MaxOutput, MaxRatio*len(src)+64)
	n, err := lz4.UncompressBlock(src, c.decompBuf[:limit])
	if err != nil {
		return nil, fmt.Errorf("lz4codec: decompress %d bytes into at most %d (MaxOutput %d): %w", len(src), limit, c.cfg.MaxOutput, err)
	}
	return c.decompBuf[:n], nil
}

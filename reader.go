// Copyright 2011 Utkan Güngördü. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lzss implements the Lempel-Ziv-Storer-Szymanski compressed
// data format, described in J. A. Storer, ``Data compression via
// textual substitution'', Journal of the ACM 29(4) (October 1984),
// (pp 928-951).
//
// The code is based on Go's compress/lzw/reader.go.
package lzss

import (
	"bufio"
	"errors"
	"io"
)

type Order int

const (
	LSB Order = iota
	MSB
)

// Keep in mind that NFlags and NOffsetBits + NLengthBits
// must be a multiple of 8 in the current implementation.
const (
	NFlags      = 8  // number of sequential flag bits
	NOffsetBits = 12 // number of bits used for relative offset
	NLengthBits = 4  // number of bits used to denote the referred chunk length
)

const (
	NReferenceBytes  = (NOffsetBits + NFlags) / 8 // number of total bytes a reference is made of (ie, offset and length pair)
	ThresholdMin     = NReferenceBytes + NFlags/8
	DefaultThreshold = ThresholdMin
)

const (
	windowLength = 1 << NOffsetBits
	flushBuffer  = 2 * windowLength
)

// FlagFunc takes the bit-vector of sequential flags and
// pos (number of flags used up until now) as parameters.
// If decoder should simply copy a single byte in
// this round (literal), this should return true.
// Otherwise, false.
// See decode for details.
type FlagFuncType func(byte, uint) bool

// RefFunc extracts chunk length and relative offset pair
// from given reference bytes. Array has the same ordering
// as the file/stream. Reqested byte ordering is available
// through parameter.
// decoder will then copy length + threshold bytes
// starting from d.output[(d.o-1)-offset].
// Note that d.output[d.o-1] is the last byte
// written by the decoder in the previous step.
// See decode in the source file for details.
type ReferenceFuncType func([]byte, Order) (int, int)

type decoder struct {
	r     io.ByteReader
	order Order
	err   error
	// output is the temporary output buffer.
	// It is flushed when it contains >= flushBuffer bytes,
	// so that there is always room to decode an entire code.
	output []byte
	o      int    // write index into output
	toRead []byte // bytes to return from Read
	i      int    // number of bytes successfully read from r

	threshold     int // minimum number of bytes in a chunk
	flagFunc      FlagFuncType
	referenceFunc ReferenceFuncType
}

// Returns the number of bytes read from given io.Reader
// Mostly for debug purposes.
func (d *decoder) Pos() int {
	return d.i
}

func (d *decoder) Read(b []byte) (int, error) {
	for {
		if len(d.toRead) > 0 {
			n := copy(b, d.toRead)
			d.toRead = d.toRead[n:]
			return n, nil
		}
		if d.err != nil {
			return 0, d.err
		}
		d.decode()
	}
	panic("unreachable")
}

func DefaultFlagFunc(flags byte, pos uint) (literal bool) {
	return flags<<pos&0x80 == 0
}

// Default functions use a format that is compatible with
// Nintendo GBA's BIOS (for LSB case), except this package assumes no header.
// See http://nocash.emubase.de/gbatek.htm#biosdecompressionfunctions for more.
func DefaultReferenceFunc(refBytes []byte, order Order) (length, offset int) {
	var lo, hi byte

	if order == LSB {
		lo = refBytes[0]
		hi = refBytes[1]
	} else {
		hi = refBytes[0]
		lo = refBytes[1]
	}

	ref := (uint16(hi) << 8) | uint16(lo)

	length = int(ref & (1<<NLengthBits - 1))
	offset = int(ref >> NLengthBits)
	return
}

// decode decompresses bytes from r and leaves them in d.toRead.
// read specifies how to decode bytes into codes.
func (d *decoder) decode() {
	defer func() {
		if d.err == io.EOF {
			d.err = io.ErrUnexpectedEOF
		}
		if d.o >= flushBuffer || d.err != nil {
			d.flush()
		}
	}()

	flags, err := d.r.ReadByte()
	if err == io.EOF {
		d.flush()
		return
	}
	if d.err != nil {
		d.err = err
		return
	}
	d.i++

	// optimize out a special case of the loop below
	if flags == 0 {
		for i := 0; i < NFlags; i++ {
			d.output[d.o], d.err = d.r.ReadByte()
			if d.err != nil {
				return
			}
			d.i++
			d.o++
		}
		return
	}

	for i := uint(0); i < NFlags; i++ {
		if d.flagFunc(flags, i) {
			d.output[d.o], d.err = d.r.ReadByte()
			if d.err != nil {
				return
			}
			d.i++
			d.o++
		} else {
			refBytes := make([]byte, NReferenceBytes)

			for i := 0; i < len(refBytes); i++ {
				if refBytes[i], d.err = d.r.ReadByte(); d.err != nil {
					return
				}
				d.i++
			}

			n, offset := d.referenceFunc(refBytes, d.order)
			if n < 0 {
				d.err = errors.New("lzss: invalid chunk length")
				return
			}

			n += d.threshold
			pos := (d.o - 1) - offset
			if offset < 0 || pos < 0 { // would never happen with a valid input.
				d.err = errors.New("lzss: relative offset out of bounds")
				return
			}
			copy(d.output[d.o:d.o+n], d.output[pos:pos+n])
			d.o += n
		}
	}
}

func (d *decoder) flush() {
	d.toRead = d.output[:d.o]
	d.o = 0
}

var errClosed = errors.New("lzss: reader/writer is closed")

func (d *decoder) Close() error {
	d.err = errClosed // in case any Reads come along
	return nil
}

// NewReader creates a new io.ReadCloser that satisfies reads by decompressing
// the data read from r.
// It is the caller's responsibility to call Close on the ReadCloser when
// finished reading.
// Threshold can't be smaller than ThresholdMin, and is typically 3.
// If you pass DefaultReferenceFunc as referenceFunc, threshold must be set to DefaultThreshold.
func NewCustomReader(r io.Reader, order Order, flagFunc FlagFuncType, referenceFunc ReferenceFuncType, threshold int) io.ReadCloser {
	d := new(decoder)

	if order != LSB && order != MSB {
		d.err = errors.New("lzss: unknown order")
		return d
	}

	if flagFunc == nil || referenceFunc == nil {
		d.err = errors.New("lzss: flagFunc and referenceFunc cannot be nil")
		return d
	}

	d.threshold = threshold
	if threshold < ThresholdMin {
		d.err = errors.New("lzss: threshold value too small")
		return d
	}

	d.order = order
	d.flagFunc = flagFunc
	d.referenceFunc = referenceFunc

	if br, ok := r.(io.ByteReader); ok {
		d.r = br
	} else {
		d.r = bufio.NewReader(r)
	}

	maxBytes := d.threshold + (1<<NLengthBits - 1) // maximum bytes in a single copy
	maxDecode := NFlags * maxBytes                 // maximum bytes output by decode in a single call
	d.output = make([]byte, flushBuffer+maxDecode)

	return d
}

func NewReader(r io.Reader, order Order) io.ReadCloser {
	return NewCustomReader(r, order, DefaultFlagFunc, DefaultReferenceFunc, DefaultThreshold)
}

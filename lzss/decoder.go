// Copyright 2011 Utkan Güngördü. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lzss implements the Lempel-Ziv-Storer-Szymanski compressed
// data format, described in J. A. Storer, ``Data compression via
// textual substitution'', Journal of the ACM 29(4) (October 1984),
// (pp 928-951).
// The code is intended to read legacy data only, so there is no encoder.
package lzss

import (
	"errors"
	"io"
)

var (
	ErrClosed       = errors.New("lzss: reader/writer is closed")
	ErrChunkLength  = errors.New("lzss: invalid chunk length")
	ErrOffset       = errors.New("lzss: relative offset out of bounds")
	ErrOrder        = errors.New("lzss: unknown order")
	ErrThreshold    = errors.New("lzss: invalid threshold value")
	ErrNilParameter = errors.New("lzss: required parameter cannot be nil")
)

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
	OffsetMask       = 1<<NLengthBits - 1
	NReferenceBytes  = (NOffsetBits + NFlags) / 8 // number of total bytes a reference is made of (ie, offset and length pair)
	ThresholdMin     = NReferenceBytes + NFlags/8
	DefaultThreshold = ThresholdMin
)

type Order int

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

type Decoder struct {
	order         Order
	threshold     int // minimum number of bytes in a chunk
	flagFunc      FlagFuncType
	referenceFunc ReferenceFuncType
}

func DefaultFlagFunc(flags byte, pos uint) (literal bool) {
	return (flags<<pos)&0x80 == 0
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

	offset = int((uint16(hi&OffsetMask) << 8) | uint16(lo))
	length = int(hi >> NLengthBits)
	return
}

// decode decompresses bytes from r and leaves them in d.toRead.
// read specifies how to decode bytes into codes.
func (d *Decoder) Decode(in []byte) ([]byte, error) {
	out := make([]byte, 0)
	l := len(in)
	r := 0
	w := 0

	for r < l {
		flags := in[r]
		r++

		// optimize a special case of the loop below
		if flags == 0 {
			if r+NFlags > l {
				return append(out, in[r:]...), io.ErrUnexpectedEOF
			}

			out = append(out, in[r:r+NFlags]...)
			r += NFlags
			w += NFlags
			continue
		}

		for i := uint(0); i < NFlags; i++ {
			if d.flagFunc(flags, i) {
				if r >= l {
					return out, io.ErrUnexpectedEOF
				}
				out = append(out, in[r])
				r++
				w++
			} else {
				if r+NReferenceBytes > l {
					return out, io.ErrUnexpectedEOF
				}
				refBytes := in[r : r+NReferenceBytes]
				r += NReferenceBytes

				n, offset := d.referenceFunc(refBytes, d.order)
				if n < 0 {
					return out, ErrChunkLength
				}

				n += d.threshold
				pos := (w - 1) - offset
				if offset < 0 || pos < 0 { // would never happen with a valid input.
					return out, ErrOffset
				}

				if pos+n > len(out) {
					return append(out, out[pos:]...), ErrChunkLength
				}
				out = append(out, out[pos:pos+n]...)
				w += n
			}
		}

	}
	return out, nil
}

func (d *Decoder) Close() error {
	return ErrClosed
}

// NewReader creates a new io.ReadCloser that satisfies reads by decompressing
// the data read from r.
// It is the caller's responsibility to call Close on the ReadCloser when
// finished reading.
// Threshold can't be smaller than ThresholdMin, and is typically 3.
// If you pass DefaultReferenceFunc as referenceFunc, threshold must be set to DefaultThreshold.
func NewCustomDecoder(order Order, flagFunc FlagFuncType, referenceFunc ReferenceFuncType, threshold int) (*Decoder, error) {
	d := new(Decoder)

	if order != LSB && order != MSB {
		return nil, ErrOrder
	}

	if flagFunc == nil || referenceFunc == nil {
		return nil, ErrNilParameter
	}

	d.threshold = threshold
	if threshold < ThresholdMin {
		return nil, ErrThreshold
	}

	d.order = order
	d.flagFunc = flagFunc
	d.referenceFunc = referenceFunc

	return d, nil
}

func NewDecoder(order Order) (*Decoder, error) {
	return NewCustomDecoder(order, DefaultFlagFunc, DefaultReferenceFunc, DefaultThreshold)
}

// Copyright 2011 Utkan Güngördü. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lzss implements the Lempel-Ziv-Storer-Szymanski compressed
// data format, described in J. A. Storer, ``Data compression via
// textual substitution'', Journal of the ACM 29(4) (October 1984),
// (pp. 928-951).
//
// The code is based on Go's compress/lzs/reader.go.
package lzss

// TODO(utkan): implelement the encoder.

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

// Keep in mind these constraints before modifying the constants defined below.
//
// - ctlWidth must be a multiple of 8 in the current implementation.
//
// - offsetWidth + sizeWidth should add up to 8*codeWidth. This can be easily mitigated to
// "multiple of 8" case by modifying the codeFuncDefault to read
// more/less than 2 bytes.
const (
	ctlWidth    = 8
	offsetWidth = 12 // number of bits used for relative offset
	sizeWidth   = 4  // number of bits used for chunk size
	threshold   = 2
	codeSize    = 2 // number of bytes used for a code
)

const (
	windowSize  = 1 << offsetWidth
	flushBuffer = 2 * windowSize
	maxBytes    = threshold + 1<<sizeWidth // maximum bytes in a single copy
	maxDecode   = ctlWidth * maxBytes      // maximum bytes output by one round of decode
)

type ctlFuncType func(byte, uint) (bool)
type codeFuncType func([]byte, Order) (int,int)

type decoder struct {
	r     io.ByteReader
	order Order
	err   error
	// output is the temporary output buffer.
	// Literal codes are accumulated from the start of the buffer.
	// It is flushed when it contains >= flushBuffer bytes,
	// so that there is always room to decode an entire code.
	output [flushBuffer + maxDecode]byte
	o      int    // write index into output
	toRead []byte // bytes to return from Read

	ctlFunc ctlFuncType
	codeFunc codeFuncType
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

func ctlFuncDefault(ctl byte, pos uint) (readOne bool) {
	return ctl << pos & 0x80 == 0
}

func codeFuncDefault(b []byte, order Order) (size, relOff int) {
	var lo, hi byte

	if order == LSB {
		lo = b[0]
		hi = b[1]
	} else {
		hi = b[0]
		lo = b[1]
	}

	code := (uint16(hi) << 8) | uint16(lo)

	size = int(code&(1<<sizeWidth-1) + threshold + 1)
	relOff = int(code>>4 + 1)
	return
}

// decode decompresses bytes from r and leaves them in d.toRead.
// read specifies how to decode bytes into codes.
// ctlWidth is the sizeWidth in bits of literal codes.
func (d *decoder) decode() {
	defer func() {
		if d.err == io.EOF {
			d.err = io.ErrUnexpectedEOF
		}
		if d.o >= flushBuffer || d.err != nil {
			d.flush()
		}
	}()

	ctl, err := d.r.ReadByte()
	if err == io.EOF {
		d.flush()
		return
	}
	if d.err != nil {
		d.err = err
		return
	}

	// optimize a special case of the loop below
	if ctl == 0 {
		for i := 0; i < ctlWidth; i++ {
			d.output[d.o], d.err = d.r.ReadByte()
			if d.err != nil {
				return
			}
			d.o++
		}
		return
	}

	for i := uint(0); i < ctlWidth; i++ {
		if d.ctlFunc(ctl,i) {
			d.output[d.o], d.err = d.r.ReadByte()
			if d.err != nil {
				return
			}
			d.o++
		} else {
			code := make([]byte, codeSize)

			for i:=0; i<len(code); i++ {
				if code[i], d.err = d.r.ReadByte(); d.err != nil {
					return
				}
			}

			n, relOff := d.codeFunc(code, d.order)

			pos := d.o - relOff
			if pos < 0 { // would never happen with a valid input.
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
// ctlFunc and codeFunc are ordinarily nil.
func NewReader(r io.Reader, order Order, ctlFunc ctlFuncType, codeFunc codeFuncType) io.ReadCloser {
	d := new(decoder)

	if order != LSB && order != MSB {
		d.err = errors.New("lzss: unknown order")
		return d
	}

	if ctlFunc == nil {
		ctlFunc = ctlFuncDefault
	}

	if codeFunc == nil {
		codeFunc = codeFuncDefault
	}

	d.order = order
	d.ctlFunc = ctlFunc
	d.codeFunc = codeFunc

	if br, ok := r.(io.ByteReader); ok {
		d.r = br
	} else {
		d.r = bufio.NewReader(r)
	}

	return d
}

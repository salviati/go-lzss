package lzss

import (
	"bytes"
	"io"
	"io/ioutil"
)

func NewCustomReader(r io.Reader, order Order, flagFunc FlagFuncType, referenceFunc ReferenceFuncType, threshold int) (io.Reader, error) {
	d, err := NewCustomDecoder(order, flagFunc, referenceFunc, threshold)
	if err != nil {
		return nil, err
	}
	
	in, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	out, err := d.Decode(in)
	return bytes.NewReader(out), err
}

func NewReader(r io.Reader, order Order) (io.Reader, error) {
	return NewCustomReader(r, order, DefaultFlagFunc, DefaultReferenceFunc, DefaultThreshold)
}


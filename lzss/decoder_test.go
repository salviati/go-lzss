package lzss

import (
	"io/ioutil"
	"os"
	"testing"
)

func read(path string, t *testing.T) []byte {
	fi, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(fi)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLorem(t *testing.T) {
	orig := read("lorem.txt", t)
	compr := read("lorem.lzss", t)
	d, err := NewDecoder(MSB)
	if err != nil {
		t.Fatal(err)
	}

	// GBA BIOS compatible data; skip the 4-bytes header.
	decompr, err := d.Decode(compr[4:])
	if err != nil {
		t.Fatal(string(decompr), err)
	}

	if len(orig) != len(decompr) {
		t.Log("Original data and decompressed data have different sizes; perhaps due to zero padding?")
	}

	if len(orig) > len(decompr) {
		t.Log("Decompressed data is smaller than the original data")
	}

	for i := 0; i < len(orig); i++ {
		if orig[i] != decompr[i] {
			t.Fatal("Original data and decompressed differ at position ", i)
		}
	}

	residue := decompr[len(orig):]
	for i := 0; i < len(residue); i++ {
		if residue[i] != 0 {
			t.Log("Non-zero byte in padding data")
		}
	}
}

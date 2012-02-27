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

func testFile(origname, comprname string, t *testing.T) {
	orig := read(origname, t)
	compr := read(comprname, t)
	d, err := NewDecoder(MSB)
	if err != nil {
		t.Fatal(err)
	}

	decompr, err := d.Decode(compr)
	if err != nil {
		t.Fatal(string(decompr), err)
	}

	if len(orig) != len(decompr) {
		t.Log("Original data and decompressed data have different sizes")
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

func TestLorem(t *testing.T) {
	testFile("lorem.txt", "lorem.lzss", t)
}

func TestPhantom(t *testing.T) {
	testFile("pg175.txt", "pg175.lzss", t)
}

package rle

import "testing"

// FuzzDecompressPlanar checks that arbitrary server data never panics. Inputs
// that once did are kept under testdata/fuzz/FuzzDecompressPlanar.
func FuzzDecompressPlanar(f *testing.F) {
	f.Add(uint8(8), uint8(8), []byte{0x10})
	f.Fuzz(func(t *testing.T, w, h uint8, src []byte) {
		width, height := int(w%64)+1, int(h%64)+1
		DecompressPlanar(make([]byte, width*height*4), width, height, src)
	})
}

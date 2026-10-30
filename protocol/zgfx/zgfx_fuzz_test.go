package zgfx

import "testing"

// FuzzDecompress checks that arbitrary server data never panics. Inputs that
// once did are kept under testdata/fuzz/FuzzDecompress.
func FuzzDecompress(f *testing.F) {
	f.Add([]byte{0xe0, 0x04, 0, 0})
	f.Fuzz(func(t *testing.T, src []byte) {
		var d Decompressor
		d.Decompress(nil, src)
	})
}

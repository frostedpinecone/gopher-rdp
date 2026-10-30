package mppc

import "testing"

func FuzzDecompress(f *testing.F) {
	f.Add([]byte{0xff, 0xff, 0x30}, byte(FlagCompressed|TypeRDP5))
	f.Fuzz(func(t *testing.T, src []byte, flags byte) {
		var d Decompressor
		d.Decompress(src, flags)
	})
}

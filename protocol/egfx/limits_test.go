package egfx

import (
	"encoding/binary"
	"log/slog"
	"testing"
)

func TestPixelAllocationLimits(t *testing.T) {
	h := NewHandler(func([]byte) error { return nil }, slog.New(slog.DiscardHandler))
	defer h.Close()
	h.handleCreateSurface([]byte{1, 0, 255, 255, 255, 255, PixelFormatXRGB8888})
	if len(h.surfaces) != 0 {
		t.Fatal("accepted oversized surface")
	}
	h.handleCreateSurface([]byte{1, 0, 4, 0, 4, 0, PixelFormatXRGB8888})
	if h.surfaces[1] == nil {
		t.Fatal("rejected small surface")
	}

	wire := make([]byte, 17)
	wire[0] = 1
	binary.LittleEndian.PutUint16(wire[9:11], 65535)
	binary.LittleEndian.PutUint16(wire[11:13], 65535)
	h.handleWireToSurface1(wire)
	if cap(h.codecBuf) != 0 {
		t.Fatal("allocated pixels for oversized rectangle")
	}
	binary.LittleEndian.PutUint16(wire[9:11], 1)
	binary.LittleEndian.PutUint16(wire[11:13], 1)
	h.handleWireToSurface1(wire) // Truncated raw pixel data must not panic.
	if cap(h.codecBuf) != 0 {
		t.Fatal("allocated pixels for truncated bitmap")
	}
	binary.LittleEndian.PutUint32(wire[13:17], 0xffffffff)
	h.handleWireToSurface1(wire)
	wire2 := make([]byte, 13)
	wire2[0], wire2[2] = 1, CodecProgressive
	binary.LittleEndian.PutUint32(wire2[9:13], 0xffffffff)
	h.handleWireToSurface2(wire2)
	fill := make([]byte, 16)
	fill[0], fill[6] = 1, 1
	binary.LittleEndian.PutUint16(fill[12:14], 65535)
	binary.LittleEndian.PutUint16(fill[14:16], 65535)
	h.handleSolidFill(fill)

	cache := make([]byte, 20)
	cache[0] = 1
	binary.LittleEndian.PutUint16(cache[16:18], 65535)
	binary.LittleEndian.PutUint16(cache[18:20], 65535)
	h.handleSurfaceToCache(cache)
	if len(h.cache) != 0 {
		t.Fatal("cached oversized rectangle")
	}
	copyRect := make([]byte, 18)
	copyRect[0], copyRect[2] = 1, 1
	binary.LittleEndian.PutUint16(copyRect[8:10], 65535)
	binary.LittleEndian.PutUint16(copyRect[10:12], 65535)
	h.handleSurfaceToSurface(copyRect)
	if cap(h.blitTmpBuf) != 0 {
		t.Fatal("allocated oversized copy buffer")
	}

	reset := make([]byte, 12)
	binary.LittleEndian.PutUint32(reset[0:4], 0xffffffff)
	binary.LittleEndian.PutUint32(reset[4:8], 0xffffffff)
	h.OnResetGraphics(func(int, int) { t.Fatal("forwarded oversized resize") })
	h.handleResetGraphics(reset)
	if h.surfaces[1] == nil {
		t.Fatal("invalid reset discarded the existing surface")
	}

	// Simulate a full budget without allocating hundreds of megabytes in a test.
	h.pixelBytes = maxGraphicsBytes
	h.handleCreateSurface([]byte{2, 0, 4, 0, 4, 0, PixelFormatXRGB8888})
	if h.surfaces[2] != nil {
		t.Fatal("surface exceeded combined budget")
	}
	binary.LittleEndian.PutUint16(cache[16:18], 2)
	binary.LittleEndian.PutUint16(cache[18:20], 2)
	h.handleSurfaceToCache(cache)
	if len(h.cache) != 0 {
		t.Fatal("cache exceeded combined budget")
	}
}

func TestPixelMemoryAccounting(t *testing.T) {
	h := NewHandler(func([]byte) error { return nil }, slog.New(slog.DiscardHandler))
	defer h.Close()
	h.handleCreateSurface([]byte{1, 0, 2, 0, 2, 0, PixelFormatXRGB8888})
	cache := make([]byte, 20)
	cache[0], cache[16], cache[18] = 1, 2, 2
	h.handleSurfaceToCache(cache)
	if h.pixelBytes != 32 {
		t.Fatalf("surface + cache = %d bytes", h.pixelBytes)
	}
	h.handleCreateSurface([]byte{1, 0, 3, 0, 3, 0, PixelFormatXRGB8888})
	h.handleSurfaceToCache(cache)
	if h.pixelBytes != 52 {
		t.Fatalf("replacement = %d bytes", h.pixelBytes)
	}
	h.handleDeleteSurface([]byte{1, 0})
	if h.pixelBytes != 16 {
		t.Fatalf("delete = %d bytes", h.pixelBytes)
	}
	h.handleCreateSurface([]byte{1, 0, 2, 0, 2, 0, PixelFormatXRGB8888})
	reset := make([]byte, 12)
	reset[0], reset[4] = 8, 8
	h.handleResetGraphics(reset)
	if h.pixelBytes != 16 {
		t.Fatalf("reset must retain cached pixels: %d bytes", h.pixelBytes)
	}
	h.handleEvictCacheEntry([]byte{0, 0})
	if h.pixelBytes != 0 {
		t.Fatalf("eviction = %d bytes", h.pixelBytes)
	}
}

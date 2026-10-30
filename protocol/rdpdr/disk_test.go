package rdpdr

import (
	"encoding/binary"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteOnClose(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		readOnly   bool
		wantStatus uint32
		wantGone   bool
	}{
		{"read-only file", "file", true, StatusAccessDenied, false},
		{"read-only root", "", true, StatusAccessDenied, false},
		{"writable root", "", false, StatusAccessDenied, false},
		{"nonempty directory", "dir", false, StatusSuccess, false},
		{"writable file", "file", false, StatusSuccess, true},
		{"empty directory", "empty", false, StatusSuccess, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"dir", "empty"} {
				if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"file", "dir/keep"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var response []byte
			h := NewHandler(func(p []byte) error { response = p; return nil }, slog.New(slog.DiscardHandler), "PC")
			h.AddDrive(1, "share", root, tc.readOnly)
			defer h.Close()
			d := h.devices[1]
			p := make([]byte, 32)
			name := encodeUTF16LE(tc.path)
			binary.LittleEndian.PutUint32(p[20:24], FileOpen)
			binary.LittleEndian.PutUint32(p[24:28], FileDeleteOnClose)
			binary.LittleEndian.PutUint32(p[28:32], uint32(len(name)))
			d.HandleIRP(h, &IORequest{DeviceID: 1, MajorFn: IrpCreate, Payload: append(p, name...)})
			if len(response) < 21 {
				t.Fatalf("short response: %x", response)
			}
			if status := binary.LittleEndian.Uint32(response[12:16]); status != tc.wantStatus {
				t.Fatalf("status = %x, want %x", status, tc.wantStatus)
			}
			if tc.wantStatus == StatusSuccess {
				d.HandleIRP(h, &IORequest{DeviceID: 1, FileID: binary.LittleEndian.Uint32(response[16:20]), MajorFn: IrpClose})
			}
			_, err := os.Stat(filepath.Join(root, tc.path))
			if os.IsNotExist(err) != tc.wantGone {
				t.Fatalf("target stat: %v; want deleted=%v", err, tc.wantGone)
			}
			if _, err := os.Stat(filepath.Join(root, "dir/keep")); err != nil {
				t.Fatalf("directory contents lost: %v", err)
			}
		})
	}
}

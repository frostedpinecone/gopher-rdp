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

func TestDriveRootContainment(t *testing.T) {
	base := t.TempDir()
	root, outside := filepath.Join(base, "share"), filepath.Join(base, "share-secret")
	for _, dir := range []string{root, outside, filepath.Join(root, "inside")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside", filepath.Join(root, "safe")); err != nil {
		t.Fatal(err)
	}
	d := NewDiskDevice(1, "share", root, false, slog.New(slog.DiscardHandler))
	t.Cleanup(d.Close)
	for _, path := range []string{"dangling", "escape/new/nested", "../share-secret/file", `..\share-secret\file`} {
		if _, ok := d.resolvePath(path); ok {
			t.Errorf("accepted escape %q", path)
		}
	}
	path, ok := d.resolvePath("safe/new/file")
	if !ok {
		t.Fatal("rejected nested creation inside share")
	}
	if _, err := d.createDirectory(filepath.Dir(path), FileOpenIf); err != nil {
		t.Fatal(err)
	}
	f, _, err := d.createFile(path, FileOpenIf)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := os.Stat(filepath.Join(root, "inside/new/file")); err != nil {
		t.Fatal(err)
	}

	// Replace a checked parent with a symlink before the actual open.
	path, ok = d.resolvePath("inside/raced")
	if !ok {
		t.Fatal("rejected local path")
	}
	if err := os.Rename(filepath.Join(root, "inside"), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	if f, _, err := d.createFile(path, FileOpenIf); err == nil {
		f.Close()
		t.Fatal("opened a path outside the share after parent replacement")
	}
}

func TestDrivePathLengthOverflow(t *testing.T) {
	var response []byte
	h := NewHandler(func(p []byte) error { response = p; return nil }, slog.New(slog.DiscardHandler), "PC")
	h.AddDrive(1, "share", t.TempDir(), false)
	defer h.Close()
	d := h.devices[1].(*DiskDevice)
	f, err := d.root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	d.files[1] = &openFile{f: f, path: ".", isDir: true}
	for _, tc := range []struct {
		name         string
		major, minor uint32
		payload      []byte
		offset       int
	}{
		{"create", IrpCreate, 0, make([]byte, 32), 28},
		{"directory", IrpDirControl, IrpMnQueryDir, make([]byte, 32), 5},
		{"rename", IrpSetInfo, 0, make([]byte, 38), 34},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.major == IrpSetInfo {
				binary.LittleEndian.PutUint32(tc.payload, FileRenameInformation)
				d.files[1].path = "file"
			}
			for _, length := range []uint32{1, 0xffffffe0, 0xfffffffa, 0xffffffff} {
				binary.LittleEndian.PutUint32(tc.payload[tc.offset:], length)
				d.HandleIRP(h, &IORequest{DeviceID: 1, FileID: 1, MajorFn: tc.major, MinorFn: tc.minor, Payload: tc.payload})
				if status := binary.LittleEndian.Uint32(response[12:16]); status != StatusInvalidParameter {
					t.Fatalf("length=%x: status=%x", length, status)
				}
			}
		})
	}
}

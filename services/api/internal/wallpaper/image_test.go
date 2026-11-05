package wallpaper

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestRealImageValidation(t *testing.T) {
	for _, tc := range []struct {
		name, typ string
		preview   bool
	}{{"image.avif", "image/avif", false}, {"4k.avif", "image/avif", false}, {"preview.webp", "image/webp", true}} {
		f, e := os.Open("testdata/" + tc.name)
		if e != nil {
			t.Fatal(e)
		}
		a, e := ValidateAsset(context.Background(), f, tc.typ, tc.preview)
		f.Close()
		if e != nil || a.Width < 1 || a.Height < 1 || len(a.SHA256) != 64 {
			t.Fatalf("real %s rejected: %v", tc.name, e)
		}
	}
	for _, tc := range []struct {
		name, typ string
		preview   bool
	}{{"nonwide.avif", "image/avif", false}, {"nonwide.webp", "image/webp", true}, {"image.webp", "image/webp", false}} {
		f, e := os.Open("testdata/" + tc.name)
		if e != nil {
			t.Fatal(e)
		}
		_, e = ValidateAsset(context.Background(), f, tc.typ, tc.preview)
		f.Close()
		if e != ErrInvalid {
			t.Fatal("non-official format accepted", tc.name)
		}
	}
	webp, e := os.ReadFile("testdata/image.webp")
	if e != nil {
		t.Fatal(e)
	}
	avif, e := os.ReadFile("testdata/image.avif")
	if e != nil {
		t.Fatal(e)
	}
	metadata := append(append([]byte{}, webp...), []byte{'E', 'X', 'I', 'F', 2, 0, 0, 0, 'G', 'P'}...)
	binary.LittleEndian.PutUint32(metadata[4:8], uint32(len(metadata)-8))
	for _, tc := range []struct {
		data    []byte
		typ     string
		preview bool
	}{{webp, "image/webp", false}, {webp, "image/avif", false}, {avif, "image/webp", false}, {avif, "image/avif", true}, {avif[:len(avif)/2], "image/avif", false}, {webp[:len(webp)/2], "image/webp", false}, {metadata, "image/webp", false}, {[]byte("\x00\x00\x00\x18ftypavif000000000000"), "image/avif", false}} {
		path := filepath.Join(t.TempDir(), "upload")
		os.WriteFile(path, tc.data, 0600)
		f, e := os.Open(path)
		if e != nil {
			t.Fatal(e)
		}
		_, e = ValidateAsset(context.Background(), f, tc.typ, tc.preview)
		f.Close()
		if e == nil {
			t.Fatal("accepted forged, corrupt, private or incompatible image")
		}
	}
	f, _ := os.Open("testdata/image.webp")
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ValidateAsset(ctx, f, "image/webp", false); e == nil {
		t.Fatal("accepted cancelled work")
	}
	s := New(nil, nil, false)
	large, e := os.CreateTemp(t.TempDir(), "oversized")
	if e != nil {
		t.Fatal(e)
	}
	defer large.Close()
	large.Truncate(MaxImageBytes + 1)
	if _, e := ValidateAsset(context.Background(), large, "image/avif", false); e != ErrInvalid {
		t.Fatal("oversized file accepted")
	}
	release1, e := s.AcquireUpload()
	if e != nil {
		t.Fatal(e)
	}
	release2, e := s.AcquireUpload()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AcquireUpload(); e != ErrBusy {
		t.Fatal("unbounded concurrent uploads")
	}
	release1()
	release2()
}

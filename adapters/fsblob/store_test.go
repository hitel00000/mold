package fsblob_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/hitel00000/mold/adapters/fsblob"
	"github.com/hitel00000/mold/storage"
)

func TestFSBlobStore_PutGetDelete(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := fsblob.New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create fsblob store: %v", err)
	}

	ctx := t.Context()
	key := "blobs/drink_images/1/image_key_test.jpg"
	content := []byte("fake image jpeg binary data")
	contentType := "image/jpeg"

	// 1. Put
	if err := bs.Put(ctx, key, bytes.NewReader(content), int64(len(content)), contentType); err != nil {
		t.Fatalf("failed to Put blob: %v", err)
	}

	// 2. Get
	r, ct, err := bs.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to Get blob: %v", err)
	}
	defer r.Close()

	if ct != contentType {
		t.Errorf("expected Content-Type %s, got %s", contentType, ct)
	}
	readBytes, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("failed to read blob stream: %v", err)
	}
	if !bytes.Equal(readBytes, content) {
		t.Errorf("expected read content to match written content")
	}

	// 3. Delete
	if err := bs.Delete(ctx, key); err != nil {
		t.Fatalf("failed to Delete blob: %v", err)
	}

	// 4. Get after Delete -> ErrNotFound
	_, _, err = bs.Get(ctx, key)
	if err == nil || !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on Get after Delete, got %v", err)
	}
}

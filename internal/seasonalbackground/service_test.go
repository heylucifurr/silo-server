package seasonalbackground

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-server/internal/blobstore/blobstoretest"
	"github.com/Silo-Server/silo-server/internal/sections/recipes"
)

// fakeEncoder stands in for libvips: it tags the bytes so a test can tell which
// upload produced an object, and fails on demand.
func fakeEncoder(tag string) func([]byte) ([]byte, error) {
	return func([]byte) ([]byte, error) { return []byte(tag), nil }
}

func newTestService(t *testing.T, tag string) (*Service, *blobstoretest.Memory) {
	t.Helper()
	store := blobstoretest.New()
	svc := NewService(store)
	svc.encode = fakeEncoder(tag)
	return svc, store
}

// A stored reference has the shape the config validates, is namespaced, and is
// the hash of the stored bytes, so replacing a background changes its URL and
// identical uploads dedupe.
func TestUploadStoreReferenceShapeAndDedupe(t *testing.T) {
	svc, store := newTestService(t, "encoded-bytes")

	ref, err := svc.Upload(t.Context(), "image/jpeg", []byte("a phone photo"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !recipes.SeasonalBackgroundRefPattern.MatchString(ref) {
		t.Fatalf("ref %q does not match the config's pattern", ref)
	}
	if !ValidRef(ref) {
		t.Fatalf("ref %q is not accepted by ValidRef", ref)
	}
	stored, ok := store.Objects[Key(ref)]
	if !ok {
		t.Fatalf("nothing stored at %q; keys: %v", Key(ref), keysOf(store))
	}
	if !bytes.Equal(stored, []byte("encoded-bytes")) {
		t.Errorf("stored %q, want the encoder's output", stored)
	}
	if !strings.HasPrefix(Key(ref), KeyPrefix+"/") {
		t.Errorf("key %q is outside the %q namespace", Key(ref), KeyPrefix)
	}

	again, err := svc.Upload(t.Context(), "image/png", []byte("a phone photo"))
	if err != nil {
		t.Fatalf("Upload again: %v", err)
	}
	if again != ref {
		t.Errorf("same bytes produced refs %q and %q; want one object", ref, again)
	}
}

// Only an image part is accepted, and bytes the encoder cannot read are the
// same refusal: neither reaches the store.
func TestUploadRefusesNonImages(t *testing.T) {
	for _, declared := range []string{"application/pdf", "text/plain", ""} {
		svc, store := newTestService(t, "encoded")
		if _, err := svc.Upload(t.Context(), declared, []byte("x")); !errors.Is(err, ErrUnsupportedImage) {
			t.Errorf("declared type %q: err = %v, want ErrUnsupportedImage", declared, err)
		}
		if len(store.Objects) != 0 {
			t.Errorf("declared type %q stored an object", declared)
		}
	}

	svc, store := newTestService(t, "encoded")
	svc.encode = func([]byte) ([]byte, error) { return nil, errors.New("not an image") }
	if _, err := svc.Upload(t.Context(), "image/webp", []byte("x")); !errors.Is(err, ErrUnsupportedImage) {
		t.Errorf("undecodable bytes: err = %v, want ErrUnsupportedImage", err)
	}
	if len(store.Objects) != 0 {
		t.Error("undecodable bytes stored an object")
	}
}

func TestUploadWithoutStorage(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.Upload(t.Context(), "image/png", []byte("x")); !errors.Is(err, ErrStorageUnavailable) {
		t.Errorf("err = %v, want ErrStorageUnavailable", err)
	}
}

// A reference a config carries must not become a storage key: anything the
// server did not write is a not-found, and the store is never asked.
func TestOpenRefusesForeignReferences(t *testing.T) {
	svc, store := newTestService(t, "encoded")
	ref, err := svc.Upload(t.Context(), "image/webp", []byte("x"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	for _, ref := range []string{"", "..", "../banana.webp", "seasonal-backgrounds/x.webp", "ABCDEF0123456789.webp", ref + ".bak"} {
		if _, _, err := svc.Open(t.Context(), ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("Open(%q) err = %v, want ErrNotFound", ref, err)
		}
	}
	if len(store.Objects) != 1 {
		t.Errorf("store holds %d objects, want the one real upload", len(store.Objects))
	}
}

func TestOpenReadsWhatUploadStored(t *testing.T) {
	svc, _ := newTestService(t, "encoded")
	ref, err := svc.Upload(t.Context(), "image/webp", []byte("x"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	reader, info, err := svc.Open(t.Context(), ref)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "encoded" {
		t.Errorf("read %q, want the stored bytes", data)
	}
	if info.Size != int64(len("encoded")) {
		t.Errorf("size = %d, want %d", info.Size, len("encoded"))
	}
}

func keysOf(store *blobstoretest.Memory) []string {
	keys := make([]string, 0, len(store.Objects))
	for key := range store.Objects {
		keys = append(keys, key)
	}
	return keys
}

// Package seasonalbackground stores the background images an administrator
// uploads for a Seasonal Picks row's web hero band.
//
// Bytes live in the same artwork blob store every other uploaded image uses
// (the local artwork root or the public bucket), so a deployment keeps them
// across a restart for the same reason it keeps artwork, and nothing new has to
// be mounted. The row config holds only a content reference, exactly as
// branding does: the reference is the hash of the stored bytes, so replacing a
// background changes the URL and no cache has to be purged.
package seasonalbackground

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/Silo-Server/silo-server/internal/blobstore"
	"github.com/Silo-Server/silo-server/internal/imageutil"
	"github.com/Silo-Server/silo-server/internal/sections/recipes"
)

const (
	// KeyPrefix is the artwork-store namespace these images live under.
	KeyPrefix = "seasonal-backgrounds"
	// MaxUploadBytes is the largest accepted upload. The image is a full-bleed
	// still behind a carousel, so it is worth more than a poster; 12 MiB also
	// covers a phone photo straight off the camera.
	MaxUploadBytes = 12 << 20
	// MaxWidth is the widest variant kept, matching the widest other uploaded
	// image. Narrower sources are re-encoded without upscaling.
	MaxWidth = 2560
	// variantKey is the name GenerateVariants gives the capped width. Written
	// out rather than built from MaxWidth, because it is a constant; the test
	// pins the two together.
	variantKey = "w2560"
)

var (
	// ErrUnsupportedImage is returned for bytes that are not a JPEG, PNG or WebP.
	ErrUnsupportedImage = errors.New("seasonalbackground: unsupported image")
	// ErrStorageUnavailable is returned when no blob store is configured.
	ErrStorageUnavailable = errors.New("seasonalbackground: storage unavailable")
	// ErrNotFound is returned for a reference the server did not write, or one
	// whose object is gone.
	ErrNotFound = errors.New("seasonalbackground: not found")
)

// Store is the slice of the artwork blob store this package writes to.
type Store interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) (io.ReadCloser, blobstore.ObjectInfo, error)
}

// Service validates, re-encodes and stores seasonal backgrounds.
type Service struct {
	store Store
	// encode produces the bytes that are stored. It is imageutil's variant
	// encoder in every real process; it is a field so the store, key and
	// validation rules can be tested without libvips.
	encode func(data []byte) ([]byte, error)
}

func NewService(store Store) *Service { return &Service{store: store, encode: encodeWebP} }

// encodeWebP re-encodes a source image as a WebP no wider than MaxWidth,
// keeping the source's own width when it is narrower.
func encodeWebP(data []byte) ([]byte, error) {
	res, err := imageutil.GenerateVariants(data, []int{MaxWidth})
	if err != nil {
		return nil, err
	}
	return variant(res, variantKey), nil
}

// HasStorage reports whether uploads can be accepted.
func (s *Service) HasStorage() bool { return s != nil && s.store != nil }

// acceptedTypes are the multipart content types an upload may declare. The
// bytes decide the format either way; a browser that sends no useful type
// (octet-stream) is still accepted when its bytes are an image.
var acceptedTypes = map[string]bool{
	"image/jpeg":               true,
	"image/png":                true,
	"image/webp":               true,
	"application/octet-stream": true,
}

// Upload re-encodes data to a size-capped WebP, stores it under a
// content-addressed key, and returns the reference to save in the row config.
func (s *Service) Upload(ctx context.Context, declaredType string, data []byte) (string, error) {
	if !s.HasStorage() {
		return "", ErrStorageUnavailable
	}
	if !acceptedTypes[declaredType] {
		return "", ErrUnsupportedImage
	}
	out, err := s.encode(data)
	if err != nil || len(out) == 0 {
		return "", ErrUnsupportedImage
	}
	// Content-address by the stored bytes, so the reference identifies what is
	// served and identical uploads dedupe.
	sum := sha256.Sum256(out)
	ref := hex.EncodeToString(sum[:])[:16] + ".webp"
	if err := s.store.Put(ctx, Key(ref), out); err != nil {
		return "", err
	}
	return ref, nil
}

// Open returns the stored bytes of a reference.
func (s *Service) Open(ctx context.Context, ref string) (io.ReadCloser, blobstore.ObjectInfo, error) {
	if !s.HasStorage() {
		return nil, blobstore.ObjectInfo{}, ErrStorageUnavailable
	}
	if !ValidRef(ref) {
		return nil, blobstore.ObjectInfo{}, ErrNotFound
	}
	reader, info, err := s.store.Get(ctx, Key(ref))
	if errors.Is(err, blobstore.ErrNotFound) {
		return nil, blobstore.ObjectInfo{}, ErrNotFound
	}
	return reader, info, err
}

// Key is the store key of a reference.
func Key(ref string) string { return KeyPrefix + "/" + ref }

// ValidRef reports whether ref has the shape this package writes. The
// reference is only ever compared, never joined to a path on its own, and a
// config that carries anything else is refused rather than turned into a key.
func ValidRef(ref string) bool { return recipes.SeasonalBackgroundRefPattern.MatchString(ref) }

// variant returns the named variant's bytes, falling back to the re-encoded
// original when the source was narrower than the cap.
func variant(res *imageutil.VariantResult, key string) []byte {
	var original []byte
	for _, v := range res.Variants {
		if v.Key == key {
			return v.Data
		}
		if v.Key == "original" {
			original = v.Data
		}
	}
	return original
}

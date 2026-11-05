package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/Silo-Server/silo-server/internal/seasonalbackground"
)

// SeasonalBackgroundHandler is the seam the v2 admin upload and the public
// background read go through. It is the only writer of the reference a
// seasonal row stores, so a row's config can only ever name bytes this server
// stored.
type SeasonalBackgroundHandler struct {
	svc *seasonalbackground.Service
}

func NewSeasonalBackgroundHandler(svc *seasonalbackground.Service) *SeasonalBackgroundHandler {
	return &SeasonalBackgroundHandler{svc: svc}
}

// HasStorage reports whether uploads can be accepted. A process without a blob
// store answers 503 rather than dropping the image.
func (h *SeasonalBackgroundHandler) HasStorage() bool { return h != nil && h.svc.HasStorage() }

// SeasonalBackgroundView is an accepted upload: the content reference the row
// config stores.
type SeasonalBackgroundView struct {
	Ref string
}

// UploadSeasonalBackground validates, re-encodes and stores one background,
// answering the reference to save in the holiday's appearance.
func (h *SeasonalBackgroundHandler) UploadSeasonalBackground(ctx context.Context, contentType string, data []byte) (SeasonalBackgroundView, error) {
	if !h.HasStorage() {
		return SeasonalBackgroundView{}, apiError(http.StatusServiceUnavailable, "unavailable", "Background upload storage is not configured")
	}
	ref, err := h.svc.Upload(ctx, contentType, data)
	switch {
	case errors.Is(err, seasonalbackground.ErrUnsupportedImage):
		return SeasonalBackgroundView{}, fieldError("file", "Unsupported image; upload a JPEG, PNG or WebP")
	case errors.Is(err, seasonalbackground.ErrStorageUnavailable):
		return SeasonalBackgroundView{}, apiError(http.StatusServiceUnavailable, "unavailable", "Background upload storage is not configured")
	case err != nil:
		return SeasonalBackgroundView{}, apiError(http.StatusInternalServerError, "internal_error", "Failed to store the background")
	}
	return SeasonalBackgroundView{Ref: ref}, nil
}

// ReadSeasonalBackground returns the stored bytes of a reference, with the
// content type to serve them as. A reference this package did not write is a
// not-found, never a storage lookup.
func (h *SeasonalBackgroundHandler) ReadSeasonalBackground(ctx context.Context, ref string) ([]byte, string, error) {
	if !h.HasStorage() {
		return nil, "", apiError(http.StatusServiceUnavailable, "unavailable", "Background storage is not configured")
	}
	reader, _, err := h.svc.Open(ctx, ref)
	switch {
	case errors.Is(err, seasonalbackground.ErrNotFound):
		return nil, "", apiError(http.StatusNotFound, "not_found", "Background not found")
	case errors.Is(err, seasonalbackground.ErrStorageUnavailable):
		return nil, "", apiError(http.StatusServiceUnavailable, "unavailable", "Background storage is not configured")
	case err != nil:
		return nil, "", apiError(http.StatusInternalServerError, "internal_error", "Failed to read the background")
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", apiError(http.StatusInternalServerError, "internal_error", "Failed to read the background")
	}
	return data, "image/webp", nil
}

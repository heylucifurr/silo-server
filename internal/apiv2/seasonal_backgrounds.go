// Seasonal Picks backgrounds: the admin upload of a holiday's hero image and
// the public read of one.
//
// The background is additive web dressing. It lives in the seasonal row's own
// config (recipes.SeasonalAppearance) and is served to whichever client asks;
// every other field a row has — its type, order, title and cards — is
// untouched, so a client that ignores it renders exactly what it rendered
// before.
package apiv2

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	"github.com/Silo-Server/silo-server/internal/seasonalbackground"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// SeasonalBackgroundService is the slice of *handlers.SeasonalBackgroundHandler
// these operations use.
type SeasonalBackgroundService interface {
	HasStorage() bool
	UploadSeasonalBackground(ctx context.Context, contentType string, data []byte) (handlers.SeasonalBackgroundView, error)
	ReadSeasonalBackground(ctx context.Context, ref string) ([]byte, string, error)
}

// seasonalBackgroundFormOverhead is the multipart framing allowance above the
// image cap.
const seasonalBackgroundFormOverhead = 1 << 20

type SeasonalBackgroundUploadForm struct {
	File huma.FormFile `form:"file" contentType:"image/png,image/jpeg,image/webp,application/octet-stream" required:"true" doc:"The background: JPEG, PNG or WebP, at most 12 MiB. It is re-encoded to WebP on the way in, so a phone photo is fine as it comes off the camera."`
}

type AdminSeasonalBackgroundUploadInput struct {
	RawBody huma.MultipartFormFiles[SeasonalBackgroundUploadForm]
}

// SeasonalBackground is an accepted upload: the reference a holiday's
// appearance stores and the URL the web loads it from.
type SeasonalBackground struct {
	Ref string `json:"ref" doc:"Content-addressed reference to save in the holiday's appearance.background" example:"5f0a1c9d3e7b2846.webp"`
	URL string `json:"url" format:"uri-reference" doc:"Stable, immutable path the image is served from; it changes when the stored bytes change" example:"/api/v2/seasonal-backgrounds/5f0a1c9d3e7b2846.webp"`
}

type AdminSeasonalBackgroundUploadOutput struct{ Body SeasonalBackground }

// seasonalBackgroundURL is where a stored background is served from. The
// reference is the hash of the bytes, so the path itself is the cache buster.
func seasonalBackgroundURL(ref string) string {
	return Prefix + "/seasonal-backgrounds/" + url.PathEscape(ref)
}

// SeasonalBackgroundRefInput names one stored background.
type SeasonalBackgroundRefInput struct {
	Ref string `path:"ref" minLength:"1" maxLength:"64" doc:"Reference returned by the admin upload" example:"5f0a1c9d3e7b2846.webp"`
}

// seasonalBackgroundProblem maps the seam's error onto a v2 problem.
func seasonalBackgroundProblem(err error) *Problem {
	apiErr, ok := err.(*handlers.APIError) //nolint:errorlint // the seam returns the value directly
	if !ok {
		return serviceProblem(err)
	}
	switch {
	case apiErr.Field != "":
		return NewProblem(TypeValidationFailed, "The request did not pass validation; see errors.").
			WithErrors(ProblemError{Location: locationBody + "." + apiErr.Field, Code: codeInvalid, Detail: apiErr.Message})
	case apiErr.Status == http.StatusNotFound:
		return NewProblem(TypeNotFound, "Background not found.")
	case apiErr.Status == http.StatusServiceUnavailable:
		return NewProblem(TypeDependencyUnavailable, apiErr.Message).WithRetryAfter(30)
	}
	return serviceProblem(err)
}

func registerSeasonalBackgrounds(reg *Registry) {
	upload := humaOp(http.MethodPost, Prefix+"/admin/sections/seasonal-backgrounds", "uploadSeasonalBackground", "admin-sections",
		"Store one Seasonal Picks background image and answer the reference a holiday's appearance saves. The image is re-encoded to WebP, addressed by the hash of the stored bytes, and served immutably from getSeasonalBackground. Until an administrator saves a row with the reference nothing shows it; replacing one leaves the previous object in place, as branding assets do.")
	upload.Errors = append(upload.Errors, http.StatusRequestEntityTooLarge)
	Register(reg, Operation{Operation: upload, Class: ClassActingAdmin, DemoRestricted: isMutatingMethod(upload.Method), ServiceBacked: true, RetrySafety: RetrySafetyNonRetryable, MaxBodyBytes: seasonalbackground.MaxUploadBytes + seasonalBackgroundFormOverhead},
		func(ctx context.Context, in *AdminSeasonalBackgroundUploadInput) (*AdminSeasonalBackgroundUploadOutput, error) {
			if reg.deps.SeasonalBackgrounds == nil {
				return nil, unavailable("seasonal backgrounds")
			}
			form := in.RawBody.Data()
			if form == nil || !form.File.IsSet {
				return nil, NewProblem(TypeValidationFailed, "The request did not pass validation; see errors.").
					WithErrors(ProblemError{Location: locationBody + ".file", Code: codeRequired, Detail: "an image file is required"})
			}
			tooLarge := NewProblem(TypePayloadTooLarge, "The image exceeds the maximum size for a background.")
			if form.File.Size > seasonalbackground.MaxUploadBytes {
				return nil, tooLarge
			}
			data, err := io.ReadAll(io.LimitReader(form.File, seasonalbackground.MaxUploadBytes+1))
			if err != nil {
				return nil, NewProblem(TypeInternalError, "An unexpected error occurred.")
			}
			if int64(len(data)) > seasonalbackground.MaxUploadBytes {
				return nil, tooLarge
			}
			view, err := reg.deps.SeasonalBackgrounds.UploadSeasonalBackground(ctx, form.File.ContentType, data)
			if err != nil {
				return nil, seasonalBackgroundProblem(err)
			}
			return &AdminSeasonalBackgroundUploadOutput{Body: SeasonalBackground{Ref: view.Ref, URL: seasonalBackgroundURL(view.Ref)}}, nil
		})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		id := "getSeasonalBackground"
		if method == http.MethodHead {
			id = "headSeasonalBackground"
		}
		entry := Operation{Operation: humaOp(method, Prefix+"/seasonal-backgrounds/{ref}", id, "home",
			"Read a stored Seasonal Picks background image. Public, because the home page loads it as a plain image; the reference is the content hash, so the bytes are cached immutably."), Class: ClassPublic, ServiceBacked: true}
		entry.Operation.Parameters = []*huma.Param{
			{Name: "ref", In: paramInPath, Required: true, Schema: &huma.Schema{Type: huma.TypeString}, Description: "Content reference returned by the admin upload."},
			{Name: ifNoneMatchField, In: paramInHeader, Schema: &huma.Schema{Type: huma.TypeString}},
		}
		headers := map[string]*huma.Param{"Content-Length": {Schema: &huma.Schema{Type: "string"}}, "ETag": {Schema: &huma.Schema{Type: "string"}}, "Cache-Control": {Schema: &huma.Schema{Type: "string"}}}
		content := map[string]*huma.MediaType{}
		if method == http.MethodGet {
			content["image/webp"] = &huma.MediaType{Schema: &huma.Schema{Type: huma.TypeString, Format: artworkBinaryFormat}}
		}
		entry.Operation.Responses = map[string]*huma.Response{
			"200": {Description: "Background image bytes (no body for HEAD)", Headers: headers, Content: content},
			"304": {Description: "Image unchanged", Headers: headers},
			"404": {Description: "Unknown reference"},
			"503": {Description: "Background storage unavailable"},
		}
		RegisterRaw(reg, RawOperation{Operation: entry, Protocol: "seasonal-background-image", Reason: "Image bytes and HEAD/conditional HTTP semantics bypass JSON encoding."}, http.HandlerFunc(reg.serveSeasonalBackground))
	}
}

func (reg *Registry) serveSeasonalBackground(w http.ResponseWriter, r *http.Request) {
	if reg.deps.SeasonalBackgrounds == nil {
		writeProblem(w, r, unavailable("seasonal backgrounds"))
		return
	}
	ref := chi.URLParam(r, "ref")
	data, media, err := reg.deps.SeasonalBackgrounds.ReadSeasonalBackground(r.Context(), ref)
	if err != nil {
		writeProblem(w, r, seasonalBackgroundProblem(err))
		return
	}
	tag := EntityTag{Opaque: ref}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", tag.String())
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if matched, p := EvaluateReadPreconditions(r.Header.Get("If-Match"), r.Header.Get(ifNoneMatchField), tag); p != nil {
		w.Header().Set("Cache-Control", "no-store")
		writeProblem(w, r, p)
		return
	} else if matched {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// seerrCapabilityID is the request_router capability sub-id of the Seerr
// plugin. Only a connection of that kind is handed to the Seerr client.
const seerrCapabilityID = "seerr"

// SeerrClient talks to Seerr's own REST API for the operations the request
// router protocol does not expose: deleting a request, removing its media,
// and checking whether a request still exists. Radarr and Sonarr are never
// called directly; the client goes through Seerr.
//
// A connection's base URL and API key come from its request_integrations row,
// so no secret is stored in code.
type SeerrClient struct {
	http *http.Client
}

// NewSeerrClient builds a Seerr client. A nil http client gets a sane default.
func NewSeerrClient(httpClient *http.Client) *SeerrClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &SeerrClient{http: httpClient}
}

// seerrRequest is one MediaRequest as Seerr reports it. Only the fields the
// host reads are decoded. Status is Seerr's MediaRequestStatus (1 pending,
// 2 approved, 3 declined); Media.Status and Media.Status4K are its MediaStatus
// for the HD and 4K variants (1 unknown, 2 pending, 3 processing,
// 4 partially available, 5 available).
type seerrRequest struct {
	ID     int  `json:"id"`
	Is4K   bool `json:"is4k"`
	Status int  `json:"status"`
	Media  *struct {
		ID       int `json:"id"`
		Status   int `json:"status"`
		Status4K int `json:"status4k"`
	} `json:"media"`
}

// Seerr's MediaRequestStatus and MediaStatus values the sync maps.
const (
	seerrRequestDeclined = 3
	seerrMediaUnknown    = 1
	seerrMediaPending    = 2
	seerrMediaProcessing = 3
	seerrMediaPartial    = 4
	seerrMediaAvailable  = 5
)

// SeerrRequestStatus is what the sync needs from one Seerr request. Found is
// false when Seerr no longer has the request (404); the sync then retires the
// request as Removed. Declined is Seerr's MediaRequestStatus "declined".
// MediaStatus and MediaStatus4K are Seerr's MediaStatus for the HD and 4K
// variants, read from the one shared media row.
type SeerrRequestStatus struct {
	Found         bool
	Declined      bool
	MediaStatus   int
	MediaStatus4K int
}

// RequestStatus reads a request's status from Seerr. A nil status means Seerr
// answered 404: the request no longer exists. An error means the lookup failed
// (offline, auth, timeout); the caller must keep the request as it is rather
// than read a failure as a deletion.
func (c *SeerrClient) RequestStatus(ctx context.Context, in Integration, externalID string) (*SeerrRequestStatus, error) {
	req, err := c.getRequest(ctx, in, externalID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, nil
	}
	status := &SeerrRequestStatus{
		Found:    true,
		Declined: req.Status == seerrRequestDeclined,
	}
	if req.Media != nil {
		status.MediaStatus = req.Media.Status
		status.MediaStatus4K = req.Media.Status4K
	}
	return status, nil
}

// TargetExists reports whether the target's request still exists in Seerr. A
// target with no external id, or a connection of another kind, is treated as
// existing so nothing is retired on a guess.
func (c *SeerrClient) TargetExists(ctx context.Context, in Integration, target Target) (bool, error) {
	if c == nil || in.CapabilityID != seerrCapabilityID {
		return true, nil
	}
	externalID := strings.TrimSpace(target.ExternalID)
	if externalID == "" {
		return true, nil
	}
	resp, err := c.do(ctx, in, http.MethodGet, "/api/v1/request/"+url.PathEscape(externalID), nil)
	if err != nil {
		return false, err
	}
	defer drain(resp.Body)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	default:
		return false, seerrHTTPError(resp)
	}
}

// DeleteTarget removes a submitted target from Seerr. The request is resolved
// first so its media entry and variant are known, then the title is dropped from
// the download service: Seerr's DELETE /media/{id}/file is its own "remove from
// Radarr or Sonarr" action and also deletes the files (Seerr exposes no switch
// for keeping them), and DELETE /request/{id} removes the request itself. The
// request goes last, so a failure leaves it in Seerr to retry, and a 404 on any
// step counts as already done, so a retry after a partial success converges.
//
// Seerr keeps ONE media row per title, shared by its HD and 4K variants
// (status/status4k, serviceId/serviceId4k), so this never deletes that row:
// deleting it for the HD variant takes the 4K variant's media id with it, and
// the 4K download is then stranded in Radarr with nothing left to name it.
func (c *SeerrClient) DeleteTarget(ctx context.Context, in Integration, target Target, opts DeleteOptions) error {
	if c == nil || in.CapabilityID != seerrCapabilityID {
		return nil
	}
	externalID := strings.TrimSpace(target.ExternalID)
	if externalID == "" {
		return nil
	}
	if opts.SiloOnly {
		// The service never calls this for a Silo-only delete; refusing here too
		// keeps a future caller from wiping a download the admin meant to keep.
		return nil
	}
	req, err := c.getRequest(ctx, in, externalID)
	if err != nil {
		return err
	}
	mediaID := 0
	if req != nil && req.Media != nil {
		mediaID = req.Media.ID
	}
	is4K := req != nil && req.Is4K
	if opts.RemoveMedia {
		if mediaID <= 0 {
			// Seerr names no media entry for the request, so nothing can remove
			// the title from Radarr or Sonarr. Refusing beats closing the Silo
			// record and leaving the download behind.
			return fmt.Errorf("seerr: request %s has no media entry, so its Radarr or Sonarr copy cannot be removed; delete it there by hand, or delete from Silo only", externalID)
		}
		if err := c.deleteMediaFile(ctx, in, mediaID, is4K); err != nil {
			return err
		}
	}
	if err := c.deleteRequest(ctx, in, externalID); err != nil {
		return err
	}
	// A destructive call worth seeing in the log: which connection lost the
	// request, and whether its download went with it.
	slog.InfoContext(ctx, "seerr: deleted request",
		"component", "requests",
		"connection", in.Name,
		"request_id", externalID,
		"media_id", mediaID,
		"removed_media", opts.RemoveMedia,
	)
	return nil
}

// getRequest reads a request, or (nil, nil) when Seerr no longer has it.
func (c *SeerrClient) getRequest(ctx context.Context, in Integration, externalID string) (*seerrRequest, error) {
	resp, err := c.do(ctx, in, http.MethodGet, "/api/v1/request/"+url.PathEscape(externalID), nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, seerrHTTPError(resp)
	}
	var req seerrRequest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&req); err != nil {
		return nil, fmt.Errorf("seerr: decode request %s: %w", externalID, err)
	}
	return &req, nil
}

func (c *SeerrClient) deleteRequest(ctx context.Context, in Integration, externalID string) error {
	return c.delete(ctx, in, "/api/v1/request/"+url.PathEscape(externalID), nil)
}

func (c *SeerrClient) deleteMediaFile(ctx context.Context, in Integration, mediaID int, is4K bool) error {
	return c.delete(ctx, in, "/api/v1/media/"+strconv.Itoa(mediaID)+"/file",
		url.Values{"is4k": []string{strconv.FormatBool(is4K)}})
}

// delete sends a DELETE and treats 404 as already done.
func (c *SeerrClient) delete(ctx context.Context, in Integration, path string, query url.Values) error {
	resp, err := c.do(ctx, in, http.MethodDelete, path, query)
	if err != nil {
		return err
	}
	defer drain(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return seerrHTTPError(resp)
	}
	return nil
}

// do builds and sends one Seerr request with the connection's credentials.
func (c *SeerrClient) do(ctx context.Context, in Integration, method, path string, query url.Values) (*http.Response, error) {
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("seerr: connection %q has no base URL", in.Name)
	}
	target := base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("seerr: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if key := strings.TrimSpace(in.APIKeyRef); key != "" {
		req.Header.Set("X-Api-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: seerr: %v", ErrIntegrationUnreachable, err)
	}
	return resp, nil
}

func seerrHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("seerr: HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("seerr: HTTP %d: %s", resp.StatusCode, msg)
}

func drain(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<16))
	_ = body.Close()
}

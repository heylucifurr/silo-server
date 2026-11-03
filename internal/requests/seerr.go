package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
// host reads are decoded.
type seerrRequest struct {
	ID    int  `json:"id"`
	Is4K  bool `json:"is4k"`
	Media *struct {
		ID int `json:"id"`
	} `json:"media"`
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
// first so its media id is known, then the requested downstream cleanup runs
// (files, then the media entry), and the request is deleted last: a failure
// leaves the request in Seerr so the admin can retry. A 404 on any step is
// treated as already done, so a retry after a partial success converges.
func (c *SeerrClient) DeleteTarget(ctx context.Context, in Integration, target Target, opts DeleteOptions) error {
	if c == nil || in.CapabilityID != seerrCapabilityID {
		return nil
	}
	externalID := strings.TrimSpace(target.ExternalID)
	if externalID == "" {
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
	if mediaID > 0 {
		if opts.DeleteFiles {
			if err := c.deleteMediaFile(ctx, in, mediaID, is4K); err != nil {
				return err
			}
		}
		if opts.RemoveMedia {
			if err := c.deleteMedia(ctx, in, mediaID); err != nil {
				return err
			}
		}
	}
	return c.deleteRequest(ctx, in, externalID)
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

func (c *SeerrClient) deleteMedia(ctx context.Context, in Integration, mediaID int) error {
	return c.delete(ctx, in, "/api/v1/media/"+strconv.Itoa(mediaID), nil)
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

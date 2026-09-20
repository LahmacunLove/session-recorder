package fileshare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pascalhuerst/session-recorder/storage"
	"github.com/rs/zerolog/log"
)

// WebDAVConfig holds configuration for Nextcloud/ownCloud WebDAV sharing
type WebDAVConfig struct {
	URL      string // WebDAV base URL, e.g. "https://cloud.example.com/remote.php/dav/files/shareuser"
	Username string
	Password string
	Folder   string // Folder path under the WebDAV root, e.g. "/SessionRecorder"
}

// WebDAVSharer uploads files to a Nextcloud/ownCloud instance via WebDAV and
// creates a public share link via the OCS Share API.
type WebDAVSharer struct {
	storage    FileStorage
	baseURL    string
	ocsBaseURL string
	username   string
	password   string
	folder     string
	client     *http.Client
}

var davFilesPathRegexp = regexp.MustCompile(`^(/remote\.php/dav/files/[^/]+)`)

// NewWebDAVSharer creates a new WebDAVSharer and ensures the target folder exists.
func NewWebDAVSharer(s FileStorage, config WebDAVConfig) (*WebDAVSharer, error) {
	folder := config.Folder
	if folder == "" {
		folder = "/SessionRecorder"
	}

	parsed, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid webdav url: %w", err)
	}

	davPath := davFilesPathRegexp.FindString(parsed.Path)
	if davPath == "" {
		return nil, fmt.Errorf("webdav url must point at a Nextcloud/ownCloud files endpoint (.../remote.php/dav/files/<user>), got %q", config.URL)
	}

	ocsBaseURL := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String() +
		"/ocs/v2.php/apps/files_sharing/api/v1/shares"

	sharer := &WebDAVSharer{
		storage:    s,
		baseURL:    strings.TrimSuffix(config.URL, "/"),
		ocsBaseURL: ocsBaseURL,
		username:   config.Username,
		password:   config.Password,
		folder:     folder,
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}

	if err := sharer.ensureFolder(context.Background(), folder); err != nil {
		return nil, fmt.Errorf("cannot create webdav share folder: %w", err)
	}

	return sharer, nil
}

func (w *WebDAVSharer) ensureFolder(ctx context.Context, folder string) error {
	req, err := http.NewRequestWithContext(ctx, "MKCOL", w.baseURL+folder, nil)
	if err != nil {
		return fmt.Errorf("failed to create mkcol request: %w", err)
	}
	req.SetBasicAuth(w.username, w.password)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach webdav server: %w", err)
	}
	defer resp.Body.Close()

	// 201 Created: folder created. 405/409: already exists.
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusMethodNotAllowed, http.StatusConflict:
		return nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mkcol failed: status %d: %s", resp.StatusCode, string(body))
	}
}

// ShareSessionFile uploads a session file to Nextcloud and returns a public share link
func (w *WebDAVSharer) ShareSessionFile(ctx context.Context, asset storage.AssetOptions, options storage.SigningOptions) (ShareResult, error) {
	reader, size, err := w.storage.GetSessionFileReader(ctx, asset)
	if err != nil {
		return ShareResult{}, fmt.Errorf("cannot get file from storage: %w", err)
	}
	defer reader.Close()

	filename := options.DownloadFilename
	if filename == "" {
		filename = string(asset.Filename)
	}

	return w.uploadAndShare(ctx, reader, size, filename, options.Expires)
}

// ShareSegmentFile uploads a segment file to Nextcloud and returns a public share link
func (w *WebDAVSharer) ShareSegmentFile(ctx context.Context, asset storage.SegmentAssetOptions, options storage.SigningOptions) (ShareResult, error) {
	reader, size, err := w.storage.GetSegmentFileReader(ctx, asset)
	if err != nil {
		return ShareResult{}, fmt.Errorf("cannot get file from storage: %w", err)
	}
	defer reader.Close()

	filename := options.DownloadFilename
	if filename == "" {
		filename = string(asset.Filename)
	}

	return w.uploadAndShare(ctx, reader, size, filename, options.Expires)
}

type ocsShareResponse struct {
	OCS struct {
		Meta struct {
			StatusCode int    `json:"statuscode"`
			Message    string `json:"message"`
		} `json:"meta"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"ocs"`
}

type ocsShareListResponse struct {
	OCS struct {
		Meta struct {
			StatusCode int    `json:"statuscode"`
			Message    string `json:"message"`
		} `json:"meta"`
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"ocs"`
}

func (w *WebDAVSharer) uploadAndShare(ctx context.Context, reader io.Reader, size int64, filename string, expires time.Duration) (ShareResult, error) {
	destPath := filepath.Join(w.folder, filepath.Base(filename))

	log.Debug().
		Str("dest-path", destPath).
		Int64("size", size).
		Msg("WebDAV: uploading file")

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, w.baseURL+destPath, reader)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to create put request: %w", err)
	}
	putReq.SetBasicAuth(w.username, w.password)
	putReq.ContentLength = size

	putResp, err := w.client.Do(putReq)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to upload to webdav: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusCreated && putResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(putResp.Body)
		return ShareResult{}, fmt.Errorf("webdav upload failed: status %d: %s", putResp.StatusCode, string(body))
	}

	return w.createShareLink(ctx, destPath, expires)
}

func (w *WebDAVSharer) createShareLink(ctx context.Context, path string, expires time.Duration) (ShareResult, error) {
	form := url.Values{}
	form.Set("path", path)
	form.Set("shareType", "3")   // public link
	form.Set("permissions", "1") // read-only
	form.Set("expireDate", time.Now().Add(expires).Format("2006-01-02"))

	shareReq, err := http.NewRequestWithContext(ctx, http.MethodPost, w.ocsBaseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to create share request: %w", err)
	}
	shareReq.SetBasicAuth(w.username, w.password)
	shareReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	shareReq.Header.Set("OCS-APIRequest", "true")
	shareReq.Header.Set("Accept", "application/json")

	shareResp, err := w.client.Do(shareReq)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to create share link: %w", err)
	}
	defer shareResp.Body.Close()

	body, err := io.ReadAll(shareResp.Body)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to read share response: %w", err)
	}

	var result ocsShareResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return ShareResult{}, fmt.Errorf("failed to parse share response: %w (body: %s)", err, string(body))
	}

	if result.OCS.Meta.StatusCode == 403 && strings.Contains(strings.ToLower(result.OCS.Meta.Message), "already shared") {
		return w.getExistingLink(ctx, path, expires)
	}

	if result.OCS.Meta.StatusCode < 200 || result.OCS.Meta.StatusCode >= 300 {
		log.Error().
			Int("status", result.OCS.Meta.StatusCode).
			Str("message", result.OCS.Meta.Message).
			Str("body", string(body)).
			Msg("Nextcloud share creation failed")
		return ShareResult{}, fmt.Errorf("nextcloud share failed: %s", result.OCS.Meta.Message)
	}

	log.Debug().
		Str("path", path).
		Str("url", result.OCS.Data.URL).
		Msg("Created Nextcloud share link")

	return ShareResult{
		URL:       result.OCS.Data.URL,
		ExpiresAt: time.Now().Add(expires),
	}, nil
}

func (w *WebDAVSharer) getExistingLink(ctx context.Context, path string, expires time.Duration) (ShareResult, error) {
	listURL := w.ocsBaseURL + "?" + url.Values{"path": {path}}.Encode()

	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to create list shares request: %w", err)
	}
	listReq.SetBasicAuth(w.username, w.password)
	listReq.Header.Set("OCS-APIRequest", "true")
	listReq.Header.Set("Accept", "application/json")

	listResp, err := w.client.Do(listReq)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to list shares: %w", err)
	}
	defer listResp.Body.Close()

	body, err := io.ReadAll(listResp.Body)
	if err != nil {
		return ShareResult{}, fmt.Errorf("failed to read list shares response: %w", err)
	}

	var result ocsShareListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return ShareResult{}, fmt.Errorf("failed to parse list shares response: %w", err)
	}

	if len(result.OCS.Data) == 0 {
		return ShareResult{}, fmt.Errorf("no existing share found for path %q", path)
	}

	return ShareResult{
		URL:       result.OCS.Data[0].URL,
		ExpiresAt: time.Now().Add(expires),
	}, nil
}

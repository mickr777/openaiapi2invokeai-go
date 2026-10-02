package invoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

type ClientOptions struct {
	AuthMode string
	Email    string
	Password string
	Version  string
}

type Client struct {
	baseURL     string
	httpClient  *http.Client
	log         *slog.Logger
	authMode    string
	email       string
	password    string
	versionMode string

	authMu sync.Mutex
	token  string

	versionMu       sync.Mutex
	detectedVersion string
}

func NewClient(baseURL string, timeout time.Duration, log *slog.Logger) *Client {
	return NewClientWithOptions(baseURL, timeout, log, ClientOptions{})
}

func NewClientWithOptions(baseURL string, timeout time.Duration, log *slog.Logger, options ClientOptions) *Client {
	authMode := strings.ToLower(strings.TrimSpace(options.AuthMode))
	if authMode == "" {
		authMode = "none"
	}
	versionMode := strings.ToLower(strings.TrimSpace(options.Version))
	if versionMode == "" {
		versionMode = "auto"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
		log: log,
		authMode: authMode,
		email: options.Email,
		password: options.Password,
		versionMode: versionMode,
	}
}

func (c *Client) ensureToken(ctx context.Context) error {
	if c.authMode != "password" {
		return nil
	}
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.token != "" {
		return nil
	}
	if c.email == "" || c.password == "" {
		return fmt.Errorf("InvokeAI email and password are required in password auth mode")
	}

	body, err := json.Marshal(map[string]any{
		"email": c.email,
		"password": c.password,
		"remember_me": false,
	})
	if err != nil {
		return fmt.Errorf("marshal InvokeAI login: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("InvokeAI login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("InvokeAI login failed (status %d)", resp.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode InvokeAI login response: %w", err)
	}
	if result.Token == "" {
		return fmt.Errorf("InvokeAI login returned no token")
	}
	c.token = result.Token
	c.log.Info("authenticated to InvokeAI", "email", c.email)
	return nil
}

func (c *Client) clearToken() {
	c.authMu.Lock()
	c.token = ""
	c.authMu.Unlock()
}

func (c *Client) bearerToken(ctx context.Context) (string, error) {
	if c.authMode != "password" {
		return "", nil
	}
	if err := c.ensureToken(ctx); err != nil {
		return "", err
	}
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.token, nil
}

func (c *Client) authorizationHeader(ctx context.Context) (http.Header, error) {
	token, err := c.bearerToken(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	return headers, nil
}

func (c *Client) doBytes(ctx context.Context, method, url string, body []byte, contentType string) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.bearerToken(ctx)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized || c.authMode != "password" || attempt == 1 {
			return resp, nil
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.clearToken()
	}
	return nil, fmt.Errorf("request retry exhausted")
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	resp, err := c.doBytes(ctx, http.MethodGet, c.baseURL+path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("InvokeAI GET %s failed (status %d): %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode InvokeAI GET %s: %w", path, err)
	}
	return nil
}

func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	if err := c.GetJSON(ctx, "/api/v1/app/version", &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Version) == "" {
		return "", fmt.Errorf("InvokeAI version endpoint returned no version")
	}
	return result.Version, nil
}

func (c *Client) VerifyVersion(ctx context.Context) (string, error) {
	switch c.versionMode {
	case "6", "7":
		return c.versionMode, nil
	case "auto":
	default:
		return "", fmt.Errorf("unsupported configured InvokeAI version mode %q", c.versionMode)
	}

	c.versionMu.Lock()
	if c.detectedVersion != "" {
		major := majorVersion(c.detectedVersion)
		c.versionMu.Unlock()
		return major, nil
	}
	c.versionMu.Unlock()

	version, err := c.GetVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("detect InvokeAI version: %w (set invoke-version to 6 or 7 to override)", err)
	}
	major := majorVersion(version)
	if major != "6" && major != "7" {
		return "", fmt.Errorf("unsupported InvokeAI version %q; supported majors are 6 and 7", version)
	}
	c.versionMu.Lock()
	c.detectedVersion = version
	c.versionMu.Unlock()
	c.log.Info("detected InvokeAI version", "version", version)
	return major, nil
}

func majorVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	end := 0
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}
	if end == 0 {
		return ""
	}
	return version[:end]
}

func (c *Client) EnqueueBatch(ctx context.Context, graph Graph) (*EnqueueBatchResponse, error) {
	reqBody := EnqueueBatchRequest{Batch: Batch{Graph: graph, Runs: 1}}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal enqueue request: %w", err)
	}
	url := fmt.Sprintf("%s/api/v1/queue/default/enqueue_batch", c.baseURL)
	c.log.Debug("enqueuing batch", "url", url)
	resp, err := c.doBytes(ctx, http.MethodPost, url, body, "application/json")
	if err != nil {
		return nil, fmt.Errorf("enqueue request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("enqueue failed (status %d): %s", resp.StatusCode, string(errBody))
	}
	var result EnqueueBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode enqueue response: %w", err)
	}
	c.log.Debug("batch enqueued", "batch_id", result.Batch.BatchID, "items", result.Enqueued)
	return &result, nil
}

func (c *Client) UploadImage(ctx context.Context, data []byte, filename string, width, height int) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", contentTypeFor(data))
	part, err := mw.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("build upload form: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write upload form: %w", err)
	}
	if width > 0 && height > 0 {
		if err := mw.WriteField("resize_to", fmt.Sprintf(`{"width":%d,"height":%d}`, width, height)); err != nil {
			return "", fmt.Errorf("write resize_to: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/api/v1/images/upload?image_category=user&is_intermediate=true", c.baseURL)
	resp, err := c.doBytes(ctx, http.MethodPost, url, body.Bytes(), mw.FormDataContentType())
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload image failed (status %d): %s", resp.StatusCode, string(errBody))
	}
	var dto struct {
		ImageName string `json:"image_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	if dto.ImageName == "" {
		return "", fmt.Errorf("upload returned no image_name")
	}
	c.log.Debug("image uploaded", "image_name", dto.ImageName, "bytes", len(data))
	return dto.ImageName, nil
}

func contentTypeFor(data []byte) string {
	if ct := http.DetectContentType(data); strings.HasPrefix(ct, "image/") {
		return ct
	}
	return "image/png"
}

func (c *Client) GetQueueItemStatus(ctx context.Context, itemID int) (*QueueItemStatus, error) {
	url := fmt.Sprintf("%s/api/v1/queue/default/i/%d", c.baseURL, itemID)
	resp, err := c.doBytes(ctx, http.MethodGet, url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("get queue item: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get queue item failed (status %d): %s", resp.StatusCode, string(errBody))
	}
	var status QueueItemStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode queue item status: %w", err)
	}
	return &status, nil
}

func (c *Client) GetImageBytes(ctx context.Context, imageName string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/api/v1/images/i/%s/full", c.baseURL, imageName)
	resp, err := c.doBytes(ctx, http.MethodGet, url, nil, "")
	if err != nil {
		return nil, "", fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch image failed (status %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read image body: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (c *Client) PollUntilComplete(ctx context.Context, itemID int, interval time.Duration) (*QueueItemStatus, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := c.GetQueueItemStatus(ctx, itemID)
			if err != nil {
				c.log.Warn("poll error", "item_id", itemID, "error", err)
				continue
			}
			c.log.Debug("poll status", "item_id", itemID, "status", status.Status)
			switch status.Status {
			case "completed", "failed", "canceled":
				return status, nil
			}
		}
	}
}

func (c *Client) GetQueueItemDetail(ctx context.Context, itemID int) (*QueueItemDetail, error) {
	url := fmt.Sprintf("%s/api/v1/queue/default/i/%d", c.baseURL, itemID)
	resp, err := c.doBytes(ctx, http.MethodGet, url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("get queue item detail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get queue item detail failed (status %d): %s", resp.StatusCode, string(errBody))
	}
	var detail QueueItemDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("decode queue item detail: %w", err)
	}
	return &detail, nil
}

func (c *Client) GetImageNames(detail *QueueItemDetail) []string {
	var names []string
	for _, out := range detail.Session.Results {
		if out.Image != nil && out.Image.ImageName != "" {
			names = append(names, out.Image.ImageName)
		}
	}
	return names
}

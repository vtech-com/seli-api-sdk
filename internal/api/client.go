package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/viper"
	"github.com/vtech-com/seli-api-sdk/internal/config"
	"github.com/vtech-com/seli-api-sdk/internal/version"
)

// defaultBaseURL mirrors api/openapi.json's servers[0].url. Used only when
// none of --api-url, SELI_API_URL, or the active profile's api_url is set.
const defaultBaseURL = "https://api.seli.vn/api/v1"

// Client is the transport for the Seli Public API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient resolves the base URL (--api-url / SELI_API_URL / profile
// api_url / spec default) and API key (SELI_API_KEY / profile api_key), per
// docs/project-context.md §3.4 and §5.1.
func NewClient() *Client {
	cfg, _ := config.Load()
	var profile *config.Profile
	if cfg != nil {
		if p, err := config.ActiveProfile(cfg); err == nil {
			profile = p
		}
	}

	baseURL := viper.GetString("api_url")
	if baseURL == "" && profile != nil {
		baseURL = profile.APIURL
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiKey := viper.GetString("api_key")
	if apiKey == "" && profile != nil {
		apiKey = profile.APIKey
	}

	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: http.DefaultClient,
	}
}

// Response is a decoded 2xx API response.
type Response struct {
	StatusCode int
	Body       []byte
	RequestID  string
}

// Get issues a GET request against path (e.g. "/health"). tenant == "" omits
// the X-Tenant-Code header entirely — callers that must enforce a required
// tenant do so before calling Get (see internal/api's *RequiresTenant consts).
func (c *Client) Get(ctx context.Context, path string, query url.Values, tenant string) (*Response, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, &Error{Code: ErrNetwork, Message: err.Error()}
	}

	requestID := newRequestID()
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("X-Request-Id", requestID)
	if tenant != "" {
		req.Header.Set("X-Tenant-Code", tenant)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, &Error{Code: ErrNetwork, Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Code: ErrNetwork, Message: err.Error()}
	}

	if resp.StatusCode >= 400 {
		return nil, decodeError(resp.StatusCode, body)
	}

	return &Response{StatusCode: resp.StatusCode, Body: body, RequestID: requestID}, nil
}

// errorEnvelope mirrors api/openapi.json's ErrorEnvelope schema.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeError(status int, body []byte) *Error {
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Error.Code == "" {
		return &Error{
			Code:       fmt.Sprintf("HTTP_%d", status),
			Message:    fmt.Sprintf("unexpected response (status %d)", status),
			HTTPStatus: status,
		}
	}
	return &Error{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: status}
}

// newRequestID generates a client-side UUIDv4 sent as X-Request-Id, echoed
// back by the server for correlation. No external dependency needed.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

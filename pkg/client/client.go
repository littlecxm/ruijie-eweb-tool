package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultTimeout = 15 * time.Second
const maxResponseSize = 4 << 20

type Client struct {
	endpoint *url.URL
	http     *http.Client
}

type options struct{ verifyTLS bool }
type Option func(*options)

// WithTLSVerification enables certificate verification. Local eWeb devices
// commonly use self-signed certificates, so verification defaults to false.
func WithTLSVerification(enabled bool) Option {
	return func(o *options) { o.verifyTLS = enabled }
}

func New(endpoint string, opts ...Option) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("endpoint must be an http(s) origin, for example https://192.168.110.1")
	}
	u.Path = ""
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: !o.verifyTLS, // Explicit compatibility default for local devices.
	}
	return &Client{endpoint: u, http: &http.Client{
		Timeout:   DefaultTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func (c *Client) Endpoint() string { return c.endpoint.String() }

func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return nil, errors.New("GET path must be relative to the device origin")
	}
	return c.request(ctx, http.MethodGet, c.endpoint.ResolveReference(u), nil)
}

// Call posts a common eWeb request. sid is empty for unauthenticated requests.
func (c *Client) Call(ctx context.Context, api string, request ApiCommonReq, sid string, result any) error {
	if api == "" || strings.ContainsAny(api, "/? #\\") || api == "." || api == ".." {
		return errors.New("invalid API name")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return errors.New("encode API request")
	}
	u := c.endpoint.ResolveReference(&url.URL{Path: "/cgi-bin/luci/api/" + api})
	if sid != "" {
		u.RawQuery = url.Values{"auth": {sid}}.Encode()
	}
	data, err := c.request(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	var response ApiResp
	if err := json.Unmarshal(data, &response); err != nil || response.Code == nil {
		return errors.New("invalid API response envelope")
	}
	if *response.Code != 0 || hasError(response.Error) {
		return &APIError{Code: *response.Code, Message: errorMessage(response)}
	}
	if result == nil {
		return nil
	}
	if len(response.Data) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) {
		return errors.New("API response is missing data")
	}
	if err := json.Unmarshal(response.Data, result); err != nil {
		return errors.New("invalid API response data")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method string, u *url.URL, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create device request")
	}
	req.Header.Set("Accept", "application/json, text/html")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error includes the auth query parameter; never expose the SID.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("device request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read device response: %w", err)
	}
	if len(data) > maxResponseSize {
		return nil, errors.New("device response exceeds size limit")
	}
	return data, nil
}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("device API error (code %d)", e.Code)
	}
	return fmt.Sprintf("device API error (code %d): %s", e.Code, e.Message)
}

func hasError(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null" && value != `""` && value != "false"
}

func errorMessage(response ApiResp) string {
	if response.Message != "" {
		return response.Message
	}
	var message string
	if json.Unmarshal(response.Error, &message) == nil {
		return message
	}
	var detail ApiErrResp
	if json.Unmarshal(response.Error, &detail) == nil {
		if detail.Msg != "" {
			return detail.Msg
		}
		return detail.Message
	}
	return ""
}

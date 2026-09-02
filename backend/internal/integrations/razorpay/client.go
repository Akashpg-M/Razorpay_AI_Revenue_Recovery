package razorpay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type APIError struct {
	StatusCode int
	Code       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Razorpay API returned HTTP %d", e.StatusCode)
}

var ErrNotConfigured = errors.New("Razorpay API credentials are not configured")
var ErrLiveModeDisabled = errors.New("Live Mode Razorpay credentials are disabled by this integration")

type Client struct {
	baseURL, keyID, keySecret string
	httpClient                *http.Client
}

func NewClient(baseURL, keyID, keySecret string) *Client {
	if baseURL == "" {
		baseURL = "https://api.razorpay.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	return &Client{baseURL: baseURL, keyID: keyID, keySecret: keySecret, httpClient: &http.Client{Timeout: 10 * time.Second}}
}
func (c *Client) WithHTTPClient(client *http.Client) *Client { c.httpClient = client; return c }
func (c *Client) Configured() bool {
	return strings.TrimSpace(c.keyID) != "" && strings.TrimSpace(c.keySecret) != ""
}
func (c *Client) Mode() string {
	if strings.HasPrefix(c.keyID, "rzp_test_") {
		return "test"
	}
	if strings.HasPrefix(c.keyID, "rzp_live_") {
		return "live"
	}
	if c.keyID == "" {
		return "unconfigured"
	}
	return "unknown"
}
func (c *Client) BaseURL() string { return c.baseURL }

type Status struct {
	Configured    bool   `json:"configured"`
	Mode          string `json:"mode"`
	Reachable     bool   `json:"reachable"`
	Authenticated bool   `json:"authenticated"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
}

func (c *Client) CheckAuthentication(ctx context.Context) Status {
	status := Status{Configured: c.Configured(), Mode: c.Mode()}
	if !status.Configured {
		status.ErrorCode = "credentials_not_configured"
		return status
	}
	if status.Mode == "live" {
		status.ErrorCode = "live_mode_disabled"
		return status
	}
	var output struct {
		Count int `json:"count"`
	}
	httpStatus, err := c.doJSONStatus(ctx, http.MethodGet, "/v1/payments?count=1", nil, &output)
	status.HTTPStatus = httpStatus
	if httpStatus > 0 {
		status.Reachable = true
	}
	if err == nil {
		status.Authenticated = true
		return status
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
		status.ErrorCode = "authentication_failed"
	} else if errors.As(err, &apiErr) {
		status.ErrorCode = "provider_http_error"
	} else {
		status.ErrorCode = "provider_unreachable"
	}
	return status
}

type PaymentLinkRequest struct {
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency"`
	Description string            `json:"description"`
	ReferenceID string            `json:"reference_id"`
	Notes       map[string]string `json:"notes,omitempty"`
	Customer    map[string]string `json:"customer,omitempty"`
	Notify      map[string]bool   `json:"notify,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
}
type PaymentLink struct {
	ID          string `json:"id"`
	ShortURL    string `json:"short_url"`
	Status      string `json:"status"`
	ReferenceID string `json:"reference_id"`
}
type PaymentStatus struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	OrderID  string `json:"order_id"`
}

func (c *Client) CreatePaymentLink(ctx context.Context, input PaymentLinkRequest) (PaymentLink, error) {
	result, _, err := c.CreatePaymentLinkWithStatus(ctx, input)
	return result, err
}
func (c *Client) CreatePaymentLinkWithStatus(ctx context.Context, input PaymentLinkRequest) (PaymentLink, int, error) {
	if c.Mode() == "live" {
		return PaymentLink{}, 0, ErrLiveModeDisabled
	}
	var result PaymentLink
	status, err := c.doJSONStatus(ctx, http.MethodPost, "/v1/payment_links", input, &result)
	return result, status, err
}
func (c *Client) FetchPaymentLink(ctx context.Context, paymentLinkID string) (PaymentLink, error) {
	result, _, err := c.FetchPaymentLinkWithStatus(ctx, paymentLinkID)
	return result, err
}
func (c *Client) FetchPaymentLinkWithStatus(ctx context.Context, paymentLinkID string) (PaymentLink, int, error) {
	if paymentLinkID == "" {
		return PaymentLink{}, 0, errors.New("payment link ID is required")
	}
	var result PaymentLink
	status, err := c.doJSONStatus(ctx, http.MethodGet, "/v1/payment_links/"+url.PathEscape(paymentLinkID), nil, &result)
	return result, status, err
}
func (c *Client) FetchPayment(ctx context.Context, paymentID string) (PaymentStatus, error) {
	if paymentID == "" {
		return PaymentStatus{}, errors.New("payment ID is required")
	}
	var result PaymentStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/payments/"+url.PathEscape(paymentID), nil, &result)
	return result, err
}
func (c *Client) doJSON(ctx context.Context, method, path string, input any, output any) error {
	_, err := c.doJSONStatus(ctx, method, path, input, output)
	return err
}
func (c *Client) doJSONStatus(ctx context.Context, method, path string, input any, output any) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	if c.Mode() == "live" {
		return 0, ErrLiveModeDisabled
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(c.keyID, c.keySecret)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, &APIError{StatusCode: resp.StatusCode, Code: providerErrorCode(data)}
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return resp.StatusCode, fmt.Errorf("decode Razorpay response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func providerErrorCode(data []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil {
		return envelope.Error.Code
	}
	return ""
}

type PaymentLinkReferenceStore interface {
	GetPaymentLink(context.Context, string) (PaymentLink, bool, error)
	SavePaymentLink(context.Context, string, PaymentLink, json.RawMessage) error
}
type PaymentLinkExecutor struct {
	client *Client
	store  PaymentLinkReferenceStore
}

func NewPaymentLinkExecutor(client *Client, store PaymentLinkReferenceStore) *PaymentLinkExecutor {
	return &PaymentLinkExecutor{client: client, store: store}
}
func (e *PaymentLinkExecutor) Execute(ctx context.Context, actionID string, input PaymentLinkRequest) (PaymentLink, error) {
	if existing, ok, err := e.store.GetPaymentLink(ctx, actionID); err != nil {
		return PaymentLink{}, err
	} else if ok {
		return existing, nil
	}
	created, err := e.client.CreatePaymentLink(ctx, input)
	if err != nil {
		return PaymentLink{}, err
	}
	raw, _ := json.Marshal(created)
	if err = e.store.SavePaymentLink(ctx, actionID, created, raw); err != nil {
		return PaymentLink{}, err
	}
	return created, nil
}
func (e *PaymentLinkExecutor) Reconcile(ctx context.Context, actionID string) (PaymentLink, error) {
	existing, ok, err := e.store.GetPaymentLink(ctx, actionID)
	if err != nil {
		return PaymentLink{}, err
	}
	if !ok {
		return PaymentLink{}, errors.New("persisted Razorpay payment link reference was not found")
	}
	return e.client.FetchPaymentLink(ctx, existing.ID)
}

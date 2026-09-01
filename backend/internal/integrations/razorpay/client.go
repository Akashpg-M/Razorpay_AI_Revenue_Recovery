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
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Razorpay API returned %d: %s", e.StatusCode, e.Body)
}

type Client struct {
	baseURL, keyID, keySecret string
	httpClient                *http.Client
}

func NewClient(baseURL, keyID, keySecret string) *Client {
	if baseURL == "" {
		baseURL = "https://api.razorpay.com"
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), keyID: keyID, keySecret: keySecret, httpClient: &http.Client{Timeout: 10 * time.Second}}
}
func (c *Client) WithHTTPClient(client *http.Client) *Client { c.httpClient = client; return c }

type PaymentLinkRequest struct {
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency"`
	Description string            `json:"description"`
	ReferenceID string            `json:"reference_id"`
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
	var result PaymentLink
	err := c.doJSON(ctx, http.MethodPost, "/v1/payment_links", input, &result)
	return result, err
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
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.keyID, c.keySecret)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Razorpay response: %w", err)
	}
	return nil
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

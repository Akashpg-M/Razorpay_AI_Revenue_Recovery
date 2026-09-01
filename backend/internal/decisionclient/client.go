package decisionclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OutcomePrediction struct {
	Action              string    `json:"action"`
	RecoveryProbability float64   `json:"recovery_probability"`
	ModelVersion        string    `json:"model_version"`
	FeatureVersion      string    `json:"feature_version"`
	PredictionTimestamp time.Time `json:"prediction_timestamp"`
}
type PredictionRequest struct {
	Context         any      `json:"context"`
	EligibleActions []string `json:"eligible_actions"`
}
type PredictionResponse struct {
	Predictions []OutcomePrediction `json:"predictions"`
}
type NaturalPredictionResponse struct {
	NaturalRecoveryProbability float64   `json:"natural_recovery_probability"`
	ModelVersion               string    `json:"model_version"`
	FeatureVersion             string    `json:"feature_version"`
	PredictionTimestamp        time.Time `json:"prediction_timestamp"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			// First model inference may include artifact deserialization and sklearn
			// initialization. Endpoint callers still provide their own deadlines.
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/health/live",
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"decision service returned status %d",
			resp.StatusCode,
		)
	}

	return nil
}

func (c *Client) PredictOutcomes(ctx context.Context, input PredictionRequest) (PredictionResponse, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return PredictionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/predict/outcomes", bytes.NewReader(encoded))
	if err != nil {
		return PredictionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PredictionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PredictionResponse{}, fmt.Errorf("decision service prediction returned status %d", resp.StatusCode)
	}
	var result PredictionResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) PredictNatural(ctx context.Context, decisionContext any) (NaturalPredictionResponse, error) {
	encoded, err := json.Marshal(map[string]any{"context": decisionContext})
	if err != nil {
		return NaturalPredictionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/predict/natural", bytes.NewReader(encoded))
	if err != nil {
		return NaturalPredictionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NaturalPredictionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NaturalPredictionResponse{}, fmt.Errorf("decision service natural prediction returned status %d", resp.StatusCode)
	}
	var result NaturalPredictionResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

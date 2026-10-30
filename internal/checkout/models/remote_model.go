package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pulsemargin/internal/checkout"
)

// RemoteModelClient calls an external ML model server via HTTP.
type RemoteModelClient struct {
	endpoint   string
	httpClient *http.Client
	fallback   checkout.MLModel
}

// NewRemoteModelClient creates a client pointing to a remote ML inference service.
func NewRemoteModelClient(endpoint string, timeout time.Duration, fallback checkout.MLModel) *RemoteModelClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return &RemoteModelClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		fallback: fallback,
	}
}

// SetHTTPClient sets a custom HTTP client (useful for mock transports and tests).
func (c *RemoteModelClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

func (c *RemoteModelClient) Name() string {
	return "remote_ml_service"
}

// Predict sends the feature set to the remote model endpoint.
func (c *RemoteModelClient) Predict(ctx context.Context, features checkout.Features) (checkout.Prediction, error) {
	body, err := json.Marshal(features)
	if err != nil {
		return c.handleFallback(ctx, features, fmt.Errorf("marshal features: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return c.handleFallback(ctx, features, fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.handleFallback(ctx, features, fmt.Errorf("remote call failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleFallback(ctx, features, fmt.Errorf("remote model returned status %d", resp.StatusCode))
	}

	var pred checkout.Prediction
	if err := json.NewDecoder(resp.Body).Decode(&pred); err != nil {
		return c.handleFallback(ctx, features, fmt.Errorf("decode remote prediction: %w", err))
	}

	return pred, nil
}

func (c *RemoteModelClient) handleFallback(ctx context.Context, features checkout.Features, originalErr error) (checkout.Prediction, error) {
	if c.fallback != nil {
		return c.fallback.Predict(ctx, features)
	}
	return checkout.Prediction{}, originalErr
}

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/devspotai/sharedkit/auth"
	"github.com/google/uuid"
	"sys-ai/llm"
)

// Contribution mirrors the relevant fields from the restaurant service's Contribution type.
type Contribution struct {
	ContributionID  uuid.UUID       `json:"contribution_id"`
	ContributorID   uuid.UUID       `json:"contributor_id"`
	EntityType      string          `json:"entity_type"`
	EntityID        *uuid.UUID      `json:"entity_id,omitempty"`
	ChangeType      string          `json:"change_type"`
	ProposedChanges json.RawMessage `json:"proposed_changes"`
	Status          string          `json:"status"`
}

type listResponse struct {
	Data  []*Contribution `json:"data"`
	Total int64           `json:"total"`
}

// RestaurantClient is an authenticated HTTP client for sys-backend-restaurant-n-shopping.
type RestaurantClient struct {
	baseURL          string
	httpClient       *http.Client
	jwtHelper        *auth.InternalJWT
	serviceAccountID string

	// cert hot-reload
	mu       sync.RWMutex
	cached   *tls.Certificate
	loadedAt time.Time
	certFile string
	keyFile  string
}

type ClientConfig struct {
	BaseURL           string
	InternalJWTSecret string
	ServiceAccountID  string
	CACertFile        string
	CertFile          string
	KeyFile           string
}

func NewRestaurantClient(cfg ClientConfig) (*RestaurantClient, error) {
	jwtHelper := auth.NewInternalJWT(auth.InternalJWTConfig{
		Secret: cfg.InternalJWTSecret,
		Expiry: 2 * time.Minute,
		Issuer: "sys-ai",
	})

	caData, err := os.ReadFile(cfg.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	c := &RestaurantClient{
		baseURL:          cfg.BaseURL,
		jwtHelper:        jwtHelper,
		serviceAccountID: cfg.ServiceAccountID,
		certFile:         cfg.CertFile,
		keyFile:          cfg.KeyFile,
	}

	tlsCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS13,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return c.getClientCert()
		},
	}
	c.httpClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	return c, nil
}

func (c *RestaurantClient) getClientCert() (*tls.Certificate, error) {
	c.mu.RLock()
	if c.cached != nil && time.Since(c.loadedAt) < 5*time.Minute {
		cert := c.cached
		c.mu.RUnlock()
		return cert, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Since(c.loadedAt) < 5*time.Minute {
		return c.cached, nil
	}
	cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		if c.cached != nil {
			return c.cached, nil
		}
		return nil, fmt.Errorf("failed to load client cert: %w", err)
	}
	c.cached = &cert
	c.loadedAt = time.Now()
	return c.cached, nil
}

func (c *RestaurantClient) mintToken() (string, error) {
	return c.jwtHelper.CreateToken(auth.CreateTokenInput{
		UserID:     c.serviceAccountID,
		Email:      "sys-ai@internal.serveyourstay.com",
		KeycloakID: c.serviceAccountID,
	})
}

// ListPendingAIReview fetches contributions awaiting AI review.
func (c *RestaurantClient) ListPendingAIReview(ctx context.Context, limit int) ([]*Contribution, error) {
	token, err := c.mintToken()
	if err != nil {
		return nil, fmt.Errorf("failed to mint service token: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/contributions?status=pending_ai_review&contributor_id=all&limit=%d", c.baseURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list pending contributions request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list contributions returned status %d: %s", resp.StatusCode, string(body))
	}

	var result listResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode contributions response: %w", err)
	}
	return result.Data, nil
}

// RecordAIReview posts the AI review result for a contribution.
func (c *RestaurantClient) RecordAIReview(ctx context.Context, contributionID uuid.UUID, result *llm.ReviewResult) error {
	token, err := c.mintToken()
	if err != nil {
		return fmt.Errorf("failed to mint service token: %w", err)
	}

	bodyBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal AI review result: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/internal/contributions/%s/ai-review", c.baseURL, contributionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("record AI review request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("record AI review returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

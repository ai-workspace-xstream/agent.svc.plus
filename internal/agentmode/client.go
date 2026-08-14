package agentmode

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

	"agent.svc.plus/internal/agentproto"
)

// ClientOptions configures the HTTP client used to communicate with the
// controller.
type ClientOptions struct {
	Timeout            time.Duration
	InsecureSkipVerify bool
	UserAgent          string
	AgentID            string
}

// Client issues authenticated requests against the controller.
type Client struct {
	baseURL   *url.URL
	token     string
	http      *http.Client
	userAgent string
	agentID   string
}

// NewClient constructs a client for the provided controller URL and token.
func NewClient(baseURL, token string, opts ClientOptions) (*Client, error) {
	trimmedURL := strings.TrimSpace(baseURL)
	if trimmedURL == "" {
		return nil, errors.New("controller url is required")
	}
	parsed, err := url.Parse(trimmedURL)
	if err != nil {
		return nil, fmt.Errorf("parse controller url: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("controller token is required")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	transport := http.DefaultTransport
	if t, ok := transport.(*http.Transport); ok {
		clone := t.Clone()
		if opts.InsecureSkipVerify {
			if clone.TLSClientConfig == nil {
				clone.TLSClientConfig = &tls.Config{}
			}
			clone.TLSClientConfig.InsecureSkipVerify = true
		}
		transport = clone
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	userAgent := strings.TrimSpace(opts.UserAgent)
	if userAgent == "" {
		userAgent = "xcontrol-agent"
	}

	return &Client{
		baseURL:   parsed,
		token:     token,
		http:      client,
		userAgent: userAgent,
		agentID:   strings.TrimSpace(opts.AgentID),
	}, nil
}

// ListClients fetches the current set of Xray clients from the controller.
func (c *Client) ListClients(ctx context.Context) (agentproto.ClientListResponse, error) {
	paths := []string{"/api/agent-server/v1/users"}
	var lastErr error

	for _, path := range paths {
		endpoint, err := url.JoinPath(c.baseURL.String(), path)
		if err != nil {
			return agentproto.ClientListResponse{}, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return agentproto.ClientListResponse{}, err
		}
		c.applyHeaders(req)
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
			_ = resp.Body.Close()
			err = fmt.Errorf("controller returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
			if resp.StatusCode == http.StatusNotFound {
				lastErr = err
				continue
			}
			return agentproto.ClientListResponse{}, err
		}

		var payload agentproto.ClientListResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()
		if err != nil {
			return agentproto.ClientListResponse{}, fmt.Errorf("decode client list: %w", err)
		}
		return payload, nil
	}

	if lastErr != nil {
		return agentproto.ClientListResponse{}, lastErr
	}
	return agentproto.ClientListResponse{}, errors.New("controller users endpoint is unavailable")
}

// ReportStatus submits the agent status report to the controller.
func (c *Client) ReportStatus(ctx context.Context, report agentproto.StatusReport) error {
	buf, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode status report: %w", err)
	}

	paths := []string{"/api/agent-server/v1/status"}
	var lastErr error

	for _, path := range paths {
		endpoint, err := url.JoinPath(c.baseURL.String(), path)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
		if err != nil {
			return err
		}
		c.applyHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
			_ = resp.Body.Close()
			err = fmt.Errorf("controller returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
			if resp.StatusCode == http.StatusNotFound {
				lastErr = err
				continue
			}
			return err
		}

		_ = resp.Body.Close()
		return nil
	}

	if lastErr != nil {
		return lastErr
	}
	return errors.New("controller status endpoint is unavailable")
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Service-Token", c.token)
	if c.agentID != "" {
		req.Header.Set("X-Agent-ID", c.agentID)
	}
	req.Header.Set("User-Agent", c.userAgent)
}

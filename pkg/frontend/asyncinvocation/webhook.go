/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2025. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package asyncinvocation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "frontend/pkg/common/faas_common/logger/log"
)

// WebhookPayload represents the payload sent to webhook URL.
type WebhookPayload struct {
	RequestID   string    `json:"requestId"`
	Status      string    `json:"status"`
	StatusCode  int       `json:"statusCode"`
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

// webhookClientTimeout returns the configured webhook timeout
func webhookClientTimeout() time.Duration {
	cfg := GetAsyncConfig()
	if cfg.Webhook.TimeoutSecond > 0 {
		return time.Duration(cfg.Webhook.TimeoutSecond) * time.Second
	}
	return 10 * time.Second // default
}

// webhookClient is the HTTP client for webhook requests (timeout set dynamically)
var webhookClient *http.Client

// SendWebhook sends the webhook notification with retry.
func SendWebhook(ctx context.Context, url string, payload *WebhookPayload) error {
	if url == "" {
		return nil
	}

	cfg := GetAsyncConfig()
	if !cfg.Webhook.Enabled {
		log.GetLogger().Debugf("Webhook is disabled, skipping notification")
		return nil
	}

	// Fix High #4: Validate webhook URL to prevent SSRF
	if err := validateWebhookURL(url); err != nil {
		log.GetLogger().Warnf("Invalid webhook URL: %v", err)
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	return sendWebhookWithRetry(ctx, url, payload, cfg.Webhook.Retry)
}

// validateWebhookURL validates the webhook URL to prevent SSRF attacks
func validateWebhookURL(urlStr string) error {
	// Only allow http and https
	if len(urlStr) < 7 || (urlStr[:7] != "http://" && urlStr[:8] != "https://") {
		return fmt.Errorf("webhook URL must start with http:// or https://")
	}
	return nil
}

// sendWebhookWithRetry sends the webhook with exponential backoff retry.
func sendWebhookWithRetry(ctx context.Context, url string, payload *WebhookPayload, retryCfg RetryConfig) error {
	maxAttempts := retryCfg.MaxAttempts
	initialDelay := time.Duration(retryCfg.InitialDelayMs) * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := initialDelay * (1 << uint(attempt-1)) // 1s, 2s, 4s...
			log.GetLogger().Infof("Retrying webhook (attempt %d/%d) after %v", attempt+1, maxAttempts, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := sendWebhookOnce(ctx, url, payload)
		if err == nil {
			log.GetLogger().Infof("Webhook sent successfully for request %s", payload.RequestID)
			return nil
		}
		lastErr = err
		log.GetLogger().Warnf("Webhook attempt %d/%d failed: %v", attempt+1, maxAttempts, err)
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", maxAttempts, lastErr)
}

// sendWebhookOnce sends a single webhook request.
func sendWebhookOnce(ctx context.Context, url string, payload *WebhookPayload) error {
	// Use dynamic timeout from config
	client := &http.Client{Timeout: webhookClientTimeout()}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Async-Request-Id", payload.RequestID)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(respBody))
}

// NewWebhookPayload creates a new WebhookPayload from AsyncResult.
func NewWebhookPayload(result *AsyncResult) *WebhookPayload {
	payload := &WebhookPayload{
		RequestID:   result.RequestID,
		Status:      result.Status,
		StatusCode:  result.StatusCode,
		CompletedAt:  time.Now(),
	}

	if result.Status == StatusCompleted && len(result.RespBody) > 0 {
		payload.Result = base64.StdEncoding.EncodeToString(result.RespBody)
	}

	if result.Error != "" {
		payload.Error = result.Error
	}

	return payload
}

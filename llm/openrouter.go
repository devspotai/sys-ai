package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReviewResult mirrors contribution.AIReviewResult in the restaurant service.
type ReviewResult struct {
	Decision       string   `json:"decision"`        // "approve" | "reject" | "flag_for_human"
	Confidence     float64  `json:"confidence"`      // 0.0 – 1.0
	Reasoning      string   `json:"reasoning"`
	FlaggedReasons []string `json:"flagged_reasons,omitempty"`
}

type Client struct {
	apiKey     string
	model      string
	url        string
	httpClient *http.Client
}

func NewClient(apiKey, model, url string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		url:    url,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// openRouterRequest is the OpenAI-compatible chat completions request body.
type openRouterRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// ReviewContribution calls the LLM to assess a proposed contribution and
// returns a structured review result.
func (c *Client) ReviewContribution(ctx context.Context, entityType, changeType string, proposedChanges json.RawMessage) (*ReviewResult, error) {
	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(entityType, changeType, proposedChanges)

	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LLM request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://serveyourstay.com")
	req.Header.Set("X-Title", "ServeYourStay Contribution Reviewer")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read LLM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var llmResp openRouterResponse
	if err := json.Unmarshal(respBytes, &llmResp); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}
	if llmResp.Error != nil {
		return nil, fmt.Errorf("LLM API error %d: %s", llmResp.Error.Code, llmResp.Error.Message)
	}
	if len(llmResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	content := llmResp.Choices[0].Message.Content
	result, err := parseReviewResult(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse review result from LLM output: %w", err)
	}
	return result, nil
}

func buildSystemPrompt() string {
	return `You are a content moderation AI for ServeYourStay, a travel platform where users
contribute information about restaurants and shopping venues.

Your job is to review proposed changes to restaurant and store entries. You must assess:
1. Whether the content is factual and plausible (not spam or fabricated)
2. Whether it follows community guidelines (no offensive content, no advertising)
3. Whether the data format is valid (prices are numeric, coordinates are realistic, etc.)
4. Whether the change is constructive (adds value, doesn't remove legitimate info)

Respond ONLY with a valid JSON object — no markdown, no explanation outside the JSON:
{
  "decision": "approve" | "reject" | "flag_for_human",
  "confidence": <float 0.0-1.0>,
  "reasoning": "<one or two sentences explaining your decision>",
  "flagged_reasons": ["<reason1>", "<reason2>"]  // only if flagging or rejecting
}

Use "approve" for clearly legitimate contributions.
Use "reject" for obvious spam, offensive content, or clearly invalid data.
Use "flag_for_human" when uncertain or the change is substantive and warrants human review.`
}

func buildUserPrompt(entityType, changeType string, proposedChanges json.RawMessage) string {
	pretty, _ := json.MarshalIndent(proposedChanges, "", "  ")
	return fmt.Sprintf(`Review this contribution:

Entity type: %s
Change type: %s

Proposed changes:
%s`, entityType, changeType, string(pretty))
}

// parseReviewResult extracts the JSON review result from the LLM's text output.
// The LLM is instructed to return only JSON, but we strip any surrounding text
// just in case the model wraps it in markdown code fences.
func parseReviewResult(content string) (*ReviewResult, error) {
	// Strip markdown code fences if present.
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var inner []string
		for i, l := range lines {
			if i == 0 {
				continue
			}
			if strings.HasPrefix(l, "```") {
				break
			}
			inner = append(inner, l)
		}
		content = strings.Join(inner, "\n")
	}

	// Find the JSON object boundaries.
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in LLM output: %q", content)
	}
	content = content[start : end+1]

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w (content: %q)", err, content)
	}

	// Validate decision field.
	switch result.Decision {
	case "approve", "reject", "flag_for_human":
	default:
		return nil, fmt.Errorf("unexpected decision value %q", result.Decision)
	}

	// Clamp confidence.
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}

	return &result, nil
}

package instantdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	client *resty.Client
}

func NewClient(appID, secret string) *Client {
	client := resty.New().
		SetBaseURL("https://api.instantdb.com").
		SetHeader("Content-Type", "application/json").
		SetAuthToken(secret).
		SetHeader("App-Id", appID)
	return &Client{client}
}

func (c *Client) SetDebug() {
	c.client.SetDebug(true)
}

type Object map[string]any

type APIError struct {
	Status  string
	Body    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error: Status: %s, Message: %s, Body: %s", e.Status, e.Message, e.Body)
}

func newAPIError(resp *resty.Response) *APIError {
	return &APIError{
		Status:  resp.Status(),
		Body:    string(resp.Body()),
		Message: extractErrorMessage(resp.Body()),
	}
}

func extractErrorMessage(body []byte) string {
	var result map[string]any
	if err := json.Unmarshal(body, &result); err == nil {
		if msg, ok := result["message"].(string); ok {
			return msg
		}
		if msg, ok := result["error"].(string); ok {
			return msg
		}
	}
	return "Unknown error"
}

func (c *Client) Query(ctx context.Context, query any, result any) error {
	body := Object{"query": query}
	req := c.client.R().SetBody(body).SetContext(ctx).SetResult(result)
	resp, err := req.Post("/admin/query")
	if err := errFromRes(resp, err); err != nil {
		return err
	}

	return nil
}

func (c *Client) Transact(ctx context.Context, steps []Transaction) error {
	var mapped []any
	for _, step := range steps {
		mapped = append(mapped, step.Body())
	}

	body := Object{"steps": mapped}
	req := c.client.R().SetBody(body).SetContext(ctx)
	resp, err := req.Post("/admin/transact")
	return errFromRes(resp, err)
}

type Transaction interface {
	Body() []any
}

type Update struct {
	Namespace string
	ID        string
	Payload   any
}

func (t Update) Body() []any {
	return []any{"update", t.Namespace, t.ID, t.Payload}
}

type Delete struct {
	Namespace string
	ID        string
}

func (t Delete) Body() []any {
	return []any{"delete", t.Namespace, t.ID}
}

type target struct {
	namespace, id string
}

type Link struct{ from, to target }

func (t *Link) From(namespace, id string) *Link {
	t.from.id = id
	t.from.namespace = namespace
	return t
}

func (t *Link) To(namespace, id string) *Link {
	t.to.id = id
	t.to.namespace = namespace
	return t
}

func (t Link) Body() []any {
	return []any{
		"link",
		t.from.namespace, t.from.id,
		Object{t.to.namespace: t.to.id},
	}
}

type Unlink struct{ from, to target }

func (t *Unlink) From(namespace, id string) *Unlink {
	t.from.id = id
	t.from.namespace = namespace
	return t
}

func (t *Unlink) To(namespace, id string) *Unlink {
	t.to.id = id
	t.to.namespace = namespace
	return t
}

func (t Unlink) Body() []any {
	return []any{
		"unlink",
		t.from.namespace, t.from.id,
		Object{t.to.namespace: t.to.id},
	}
}

func (c *Client) CreateToken(email string) (string, error) {
	body := map[string]string{"email": email}
	resp, err := c.client.R().SetBody(body).Post("/admin/refresh_tokens")
	if err := errFromRes(resp, err); err != nil {
		return "", err
	}

	var result struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return result.RefreshToken, nil
}

func (c *Client) VerifyToken(refreshToken string) (json.RawMessage, error) {
	body := map[string]string{
		"app-id":        c.client.Header.Get("App-Id"),
		"refresh-token": refreshToken,
	}
	resp, err := c.client.R().SetBody(body).Post("/runtime/auth/verify_refresh_token")
	if err := errFromRes(resp, err); err != nil {
		return nil, err
	}

	return resp.Body(), nil
}

func (c *Client) AsEmail(email string) {
	c.client = c.client.SetHeader("as-email", email)
}

func (c *Client) AsToken(token string) {
	c.client = c.client.SetHeader("as-token", token)
}

func (c *Client) AsGuest() {
	c.client = c.client.SetHeader("as-guest", "true")
}

func errFromRes(res *resty.Response, err error) error {
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	if res.IsError() {
		return newAPIError(res)
	}

	return nil
}

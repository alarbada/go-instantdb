package idb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
)

func NewID() string {
	return uuid.NewString()
}

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

func (c *Client) SetDebug() *Client {
	c.client.SetDebug(true)
	return c
}

// O ia an instaml / instaql type helper.
type O map[string]any

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
	body := O{"query": query}
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

	body := O{"steps": mapped}
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
		O{t.to.namespace: t.to.id},
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
		O{t.to.namespace: t.to.id},
	}
}

// User represents an instantdb user.
type User struct {
	ID           string `json:"id"`
	AppID        string `json:"app_id"`
	Email        string `json:"email"`
	CreatedAt    string `json:"created_at"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Client) CreateToken(ctx context.Context, email string) (*User, error) {
	body := map[string]string{"email": email}
	resp, err := c.client.R().SetBody(body).
		SetContext(ctx).
		Post("/admin/refresh_tokens")
	if err := errFromRes(resp, err); err != nil {
		return nil, err
	}

	var result struct {
		User User
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result.User, nil
}

func (c *Client) VerifyToken(ctx context.Context, refreshToken string) (*User, error) {
	body := map[string]string{
		"app-id":        c.client.Header.Get("App-Id"),
		"refresh-token": refreshToken,
	}
	resp, err := c.client.R().SetBody(body).
		SetContext(ctx).
		Post("/runtime/auth/verify_refresh_token")
	if err := errFromRes(resp, err); err != nil {
		return nil, err
	}

	var result struct {
		User User
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result.User, nil
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

func Lookup(prop string, val any) string {
	return fmt.Sprintf("lookup__%s__\"%v\"", prop, val)
}

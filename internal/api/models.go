package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// Health is the GET /health response body — an auth-path canary.
type Health struct {
	OK        bool   `json:"ok"`
	Timestamp string `json:"timestamp"`
}

// Member is a tenant membership record. JSON tags match api/openapi.json's
// camelCase field names exactly: writeEnvelope marshals this struct directly
// for CLI output, there is no separate display-model layer in this repo.
//
// The six pointer fields are in the schema's required[] but typed
// ["string","null"] — the key is always present, the value may be null.
type Member struct {
	ID           string  `json:"id"`
	UserID       string  `json:"userId"`
	Level        string  `json:"level"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"createdAt"`
	DisplayName  *string `json:"displayName"`
	AvatarURL    *string `json:"avatarUrl"`
	ContactEmail *string `json:"contactEmail"`
	ContactPhone *string `json:"contactPhone"`
	JobTitle     *string `json:"jobTitle"`
	Department   *string `json:"department"`
	MemberCode   *string `json:"memberCode"`
}

// Pagination mirrors the GET /members response's meta.pagination block.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// MembersMeta is the meta block of the GET /members response.
type MembersMeta struct {
	Pagination Pagination `json:"pagination"`
}

type healthEnvelope struct {
	Data Health `json:"data"`
}

// GetHealth calls GET /health. tenant must already be resolved by the
// caller — HealthRequiresTenant is true.
func (c *Client) GetHealth(ctx context.Context, tenant string) (*Health, error) {
	resp, err := c.Get(ctx, "/health", nil, tenant)
	if err != nil {
		return nil, err
	}
	var env healthEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, &Error{Code: ErrNetwork, Message: "malformed response body: " + err.Error()}
	}
	return &env.Data, nil
}

type memberEnvelope struct {
	Data Member `json:"data"`
}

type membersListEnvelope struct {
	Data []Member    `json:"data"`
	Meta MembersMeta `json:"meta"`
}

// ListMembersParams are the GET /members query parameters. Zero values mean
// "omit" — the server applies its own defaults (page=1, pageSize=20).
type ListMembersParams struct {
	Page     int
	PageSize int
	Status   string
}

// ListMembers calls GET /members. tenant must already be resolved by the
// caller — MembersRequiresTenant is true.
func (c *Client) ListMembers(ctx context.Context, tenant string, params ListMembersParams) ([]Member, *MembersMeta, error) {
	q := url.Values{}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.PageSize > 0 {
		q.Set("pageSize", strconv.Itoa(params.PageSize))
	}
	if params.Status != "" {
		q.Set("status", params.Status)
	}

	resp, err := c.Get(ctx, "/members", q, tenant)
	if err != nil {
		return nil, nil, err
	}
	var env membersListEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, nil, &Error{Code: ErrNetwork, Message: "malformed response body: " + err.Error()}
	}
	return env.Data, &env.Meta, nil
}

// GetMember calls GET /members/{id}. tenant must already be resolved by the
// caller — MembersRequiresTenant is true.
func (c *Client) GetMember(ctx context.Context, tenant, id string) (*Member, error) {
	resp, err := c.Get(ctx, "/members/"+id, nil, tenant)
	if err != nil {
		return nil, err
	}
	var env memberEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, &Error{Code: ErrNetwork, Message: "malformed response body: " + err.Error()}
	}
	return &env.Data, nil
}

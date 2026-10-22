package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Health is the GET /api/health response body — an auth-path canary.
type Health struct {
	OK        bool   `json:"ok"`
	Timestamp string `json:"timestamp"`
}

// Member is a tenant membership record. JSON tags match api/openapi.json's
// camelCase field names exactly: the command marshals this struct directly
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

// Pagination mirrors the GET /api/members response's meta.pagination block.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// MembersPage is one page of GET /api/members.
type MembersPage struct {
	Members []Member
	// Pagination is meta.pagination decoded, for the loop that pages.
	Pagination Pagination
	// Meta is the API's own meta object, verbatim. The command prints it, so a
	// key the API adds to meta reaches the caller without a release here.
	Meta json.RawMessage
}

type healthEnvelope struct {
	Data Health `json:"data"`
}

// GetHealth calls GET /api/health. tenant must already be resolved by the
// caller — HealthRequiresTenant is true.
func (c *Client) GetHealth(ctx context.Context, tenant string) (*Health, error) {
	resp, err := c.Do(ctx, http.MethodGet, "/api/health", nil, tenant)
	if err != nil {
		return nil, err
	}
	var env healthEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, malformedBody(resp, err)
	}
	return &env.Data, nil
}

type memberEnvelope struct {
	Data Member `json:"data"`
}

type membersListEnvelope struct {
	Data []Member        `json:"data"`
	Meta json.RawMessage `json:"meta"`
}

type membersListMeta struct {
	Pagination Pagination `json:"pagination"`
}

// ListMembersParams are the GET /api/members query parameters. Zero values
// mean "omit" — the server applies its own defaults (page=1, pageSize=20).
type ListMembersParams struct {
	Page     int
	PageSize int
	Status   string
}

// ListMembers calls GET /api/members. tenant must already be resolved by the
// caller — MembersRequiresTenant is true.
func (c *Client) ListMembers(ctx context.Context, tenant string, params ListMembersParams) (*MembersPage, error) {
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

	path := "/api/members"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.Do(ctx, http.MethodGet, path, nil, tenant)
	if err != nil {
		return nil, err
	}
	var env membersListEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, malformedBody(resp, err)
	}
	page := &MembersPage{Members: env.Data, Meta: env.Meta}
	if len(env.Meta) > 0 {
		var meta membersListMeta
		if err := json.Unmarshal(env.Meta, &meta); err != nil {
			return nil, malformedBody(resp, err)
		}
		page.Pagination = meta.Pagination
	}
	return page, nil
}

// ListAllMembers is ListMembers repeated, from page 1, until the last page
// meta.pagination reports (params.Page is ignored). The result holds every
// member fetched, and the meta and pagination of the last page.
//
// It is the one place a request loop is allowed: it exists because the caller
// asked for --all, and it stops at totalPages. A failure after at least one page
// returns the pages fetched so far together with the error, so the caller can
// show what it has instead of discarding it; a failure on page 1 returns a nil
// page.
func (c *Client) ListAllMembers(ctx context.Context, tenant string, params ListMembersParams) (*MembersPage, error) {
	var all *MembersPage
	for page := 1; ; page++ {
		params.Page = page
		p, err := c.ListMembers(ctx, tenant, params)
		if err != nil {
			return all, err
		}
		if all == nil {
			all = p
		} else {
			all.Members = append(all.Members, p.Members...)
			all.Pagination = p.Pagination
			all.Meta = p.Meta
		}
		if page >= p.Pagination.TotalPages {
			return all, nil
		}
	}
}

// GetMember calls GET /api/members/{id}. tenant must already be resolved by
// the caller — MembersRequiresTenant is true.
func (c *Client) GetMember(ctx context.Context, tenant, id string) (*Member, error) {
	resp, err := c.Do(ctx, http.MethodGet, "/api/members/"+url.PathEscape(id), nil, tenant)
	if err != nil {
		return nil, err
	}
	var env memberEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, malformedBody(resp, err)
	}
	return &env.Data, nil
}

// malformedBody is the error for a 2xx whose body is not the JSON the spec
// promises. It keeps the request id and the body, so the failure can be
// correlated and shown without re-running with --verbose.
func malformedBody(resp *Response, err error) *APIError {
	return &APIError{
		Code:       ErrNetwork,
		Message:    "malformed response body: " + err.Error(),
		RequestID:  resp.RequestID,
		HTTPStatus: resp.StatusCode,
		RawBody:    resp.Body,
		cause:      err,
	}
}

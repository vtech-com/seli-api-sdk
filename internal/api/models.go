package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Pagination is meta.pagination decoded, for the loop that pages. It is the
	// zero value when the response carried none; hasPagination tells the two apart.
	Pagination Pagination
	// Meta is the API's own meta object, verbatim. The command prints it, so a
	// key the API adds to meta reaches the caller without a release here.
	Meta json.RawMessage

	hasPagination bool      // meta.pagination was present and not null
	resp          *Response // the reply this page was decoded from, for error reporting
}

// maxAllPages is the most pages ListAllMembers will fetch: 1000 pages of 100 is
// 100000 members. It trusts the server's totalPages to know when to stop, and a
// server (or a bug) that reports an enormous one must not make a command an
// unbounded loop.
const maxAllPages = 1000

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
	// A pointer, so an absent or null pagination is not mistaken for a real
	// {page: 0, totalPages: 0} — which would read as "that was the last page".
	Pagination *Pagination `json:"pagination"`
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
	page := &MembersPage{Members: env.Data, Meta: env.Meta, resp: resp}
	if len(env.Meta) > 0 {
		var meta membersListMeta
		if err := json.Unmarshal(env.Meta, &meta); err != nil {
			return nil, malformedBody(resp, err)
		}
		if meta.Pagination != nil {
			page.Pagination = *meta.Pagination
			page.hasPagination = true
		}
	}
	return page, nil
}

// ListAllMembers is ListMembers repeated, from page 1, until the last page
// meta.pagination reports (params.Page is ignored). The result holds every
// member fetched, and the meta and pagination of the last page.
//
// It is the one place a request loop is allowed: it exists because the caller
// asked for --all, it stops at totalPages, and it never fetches more than
// maxAllPages pages. A failure after at least one page returns the pages fetched
// so far together with the error, so the caller can show what it has instead of
// discarding it; a failure on page 1 returns a nil page.
//
// Two conditions are errors, not quiet stops, because stopping would pass a
// prefix off as the whole list: a response with no meta.pagination (the loop
// cannot tell where the last page is), and a list longer than the page cap.
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
			all.hasPagination = p.hasPagination
			all.resp = p.resp
		}
		if !p.hasPagination {
			return all, noPaginationError(p.resp)
		}
		if page >= p.Pagination.TotalPages {
			return all, nil
		}
		if page >= maxAllPages {
			return all, pageCapError(p)
		}
	}
}

// noPaginationError is the error for a list reply that has no meta.pagination
// during a sweep, which needs it to know when to stop.
func noPaginationError(resp *Response) *APIError {
	return &APIError{
		Code:       ErrNetwork,
		Message:    "malformed response body: it has no meta.pagination, so --all cannot tell where the last page is; the rows fetched so far are returned",
		RequestID:  resp.RequestID,
		HTTPStatus: resp.StatusCode,
		RawBody:    resp.Body,
	}
}

// pageCapError is the error for a sweep that reached maxAllPages with pages
// still to go.
func pageCapError(last *MembersPage) *APIError {
	return &APIError{
		Code: ErrPageLimit,
		Message: fmt.Sprintf("stopped after %d pages: the API reports %d pages (%d members in all). The rows fetched so far are returned; narrow the list with --status, or read further pages with --page",
			maxAllPages, last.Pagination.TotalPages, last.Pagination.Total),
		RequestID: last.resp.RequestID,
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

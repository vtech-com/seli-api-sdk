package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const membersListBody = `{
  "data": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "userId": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "level": "member",
      "status": "active",
      "createdAt": "2026-07-24T09:30:00.000Z",
      "displayName": "Test Member",
      "avatarUrl": null,
      "contactEmail": "member@example.test",
      "contactPhone": null,
      "jobTitle": "Engineer",
      "department": "Engineering",
      "memberCode": "M-001"
    }
  ],
  "meta": { "pagination": { "page": 1, "pageSize": 20, "total": 1, "totalPages": 1 } }
}`

func TestListMembersSuccess(t *testing.T) {
	var gotPath, gotPage, gotPageSize, gotStatus, gotTenant string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPage = r.URL.Query().Get("page")
		gotPageSize = r.URL.Query().Get("pageSize")
		gotStatus = r.URL.Query().Get("status")
		gotTenant = r.Header.Get("X-Tenant-Code")
		_, _ = w.Write([]byte(membersListBody))
	})

	page, err := c.ListMembers(context.Background(), "acme", ListMembersParams{Page: 2, PageSize: 20, Status: "active"})
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if gotPath != "/api/members" {
		t.Errorf("path: got %q, want %q", gotPath, "/api/members")
	}
	if gotPage != "2" || gotPageSize != "20" || gotStatus != "active" {
		t.Errorf("query = page %q pageSize %q status %q", gotPage, gotPageSize, gotStatus)
	}
	if gotTenant != "acme" {
		t.Errorf("tenant header: got %q, want %q", gotTenant, "acme")
	}
	if len(page.Members) != 1 {
		t.Fatalf("members length: got %d, want 1", len(page.Members))
	}
	m := page.Members[0]
	if m.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("member ID: got %q", m.ID)
	}
	if m.AvatarURL != nil {
		t.Errorf("AvatarURL: got %v, want nil", m.AvatarURL)
	}
	if m.ContactEmail == nil || *m.ContactEmail != "member@example.test" {
		t.Errorf("ContactEmail: got %v", m.ContactEmail)
	}
	if page.Pagination != (Pagination{Page: 1, PageSize: 20, Total: 1, TotalPages: 1}) {
		t.Errorf("Pagination = %+v", page.Pagination)
	}
}

// The API's meta reaches the caller as the API sent it, including a key this
// package has never heard of.
func TestListMembersKeepsTheAPIsMetaVerbatim(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"meta":{"pagination":{"page":1,"pageSize":20,"total":0,"totalPages":0},"future":{"k":1}}}`))
	})

	page, err := c.ListMembers(context.Background(), "acme", ListMembersParams{})
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	const want = `{"pagination":{"page":1,"pageSize":20,"total":0,"totalPages":0},"future":{"k":1}}`
	if string(page.Meta) != want {
		t.Errorf("Meta = %s, want %s", page.Meta, want)
	}
	if page.Members == nil {
		t.Error("Members is nil for an empty page; want an empty slice from the decoder")
	}
}

func TestListMembersOmitsUnsetQueryParams(t *testing.T) {
	var rawQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(membersListBody))
	})

	if _, err := c.ListMembers(context.Background(), "acme", ListMembersParams{}); err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if rawQuery != "" {
		t.Errorf("no query params were set, but the request carried %q", rawQuery)
	}
}

// pagesServer serves `total` pages of `perPage` members each, and fails page
// `failOn` (0: never) with the given status.
func pagesServer(t *testing.T, total, perPage, failOn, failStatus int, pagesSeen *[]int) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		*pagesSeen = append(*pagesSeen, page)
		if page == failOn {
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(`{"error":{"code":"E0000_UNKNOWN","message":"boom"}}`))
			return
		}
		rows := ""
		for i := 0; i < perPage; i++ {
			if i > 0 {
				rows += ","
			}
			rows += fmt.Sprintf(`{"id":"p%d-%d","userId":"u","level":"member","status":"active","createdAt":"t","displayName":null,"avatarUrl":null,"contactEmail":null,"contactPhone":null,"jobTitle":null,"department":null,"memberCode":null}`, page, i)
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s],"meta":{"pagination":{"page":%d,"pageSize":%d,"total":%d,"totalPages":%d}}}`,
			rows, page, perPage, total*perPage, total)
	})
}

func TestListAllMembersWalksEveryPageOnce(t *testing.T) {
	var seen []int
	c := pagesServer(t, 3, 2, 0, 0, &seen)

	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{PageSize: 2, Page: 99})
	if err != nil {
		t.Fatalf("ListAllMembers: %v", err)
	}
	if len(page.Members) != 6 {
		t.Errorf("members = %d, want 6", len(page.Members))
	}
	if fmt.Sprint(seen) != "[1 2 3]" {
		t.Errorf("pages requested = %v, want [1 2 3]: each page exactly once, from page 1 (params.Page is ignored)", seen)
	}
	if page.Pagination.Page != 3 || page.Pagination.TotalPages != 3 {
		t.Errorf("Pagination = %+v, want the last page's", page.Pagination)
	}
	if page.Members[0].ID != "p1-0" || page.Members[5].ID != "p3-1" {
		t.Errorf("members are out of order: first %q last %q", page.Members[0].ID, page.Members[5].ID)
	}
}

func TestListAllMembersSinglePage(t *testing.T) {
	var seen []int
	c := pagesServer(t, 1, 3, 0, 0, &seen)
	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{})
	if err != nil {
		t.Fatalf("ListAllMembers: %v", err)
	}
	if len(page.Members) != 3 || len(seen) != 1 {
		t.Errorf("members = %d, requests = %d", len(page.Members), len(seen))
	}
}

func TestListAllMembersEmptyTenantStopsAfterOnePage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"meta":{"pagination":{"page":1,"pageSize":20,"total":0,"totalPages":0}}}`))
	})
	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{})
	if err != nil {
		t.Fatalf("ListAllMembers: %v", err)
	}
	if len(page.Members) != 0 {
		t.Errorf("members = %d, want 0", len(page.Members))
	}
}

// A failure after page 1 hands back what was fetched, with the error, so the
// caller can show the rows instead of discarding them.
func TestListAllMembersReturnsThePagesFetchedBeforeAFailure(t *testing.T) {
	var seen []int
	c := pagesServer(t, 4, 2, 3, http.StatusTooManyRequests, &seen)

	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{PageSize: 2})
	if err == nil {
		t.Fatal("expected the page-3 failure")
	}
	if page == nil || len(page.Members) != 4 {
		t.Fatalf("partial page = %+v, want the 4 members of pages 1-2", page)
	}
	if page.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want the last page that succeeded (2)", page.Pagination.Page)
	}
	if got := ExitCodeFor(err); got != 7 {
		t.Errorf("exit code = %d, want 7", got)
	}
	if fmt.Sprint(seen) != "[1 2 3]" {
		t.Errorf("pages requested = %v: nothing after the failure should be asked for", seen)
	}
}

func TestListAllMembersFailureOnPageOneReturnsNoPage(t *testing.T) {
	var seen []int
	c := pagesServer(t, 3, 2, 1, http.StatusForbidden, &seen)

	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if page != nil {
		t.Errorf("page = %+v, want nil: nothing was fetched", page)
	}
}

const oneMemberJSON = `{"id":"m1","userId":"u","level":"member","status":"active","createdAt":"t","displayName":null,"avatarUrl":null,"contactEmail":null,"contactPhone":null,"jobTitle":null,"department":null,"memberCode":null}`

// --all trusts the server's totalPages to know when to stop. A server (or a bug)
// that reports an enormous one must not turn a command into an unbounded loop:
// the sweep stops at a hard page cap and hands back what it has, with an error.
func TestListAllMembersStopsAtAHardPageCap(t *testing.T) {
	var requests atomic.Int64
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// A backstop so that code without a cap ends here, not after 100000 pages.
		if requests.Add(1) > 1500 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"E0000_UNKNOWN","message":"backstop"}}`))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		_, _ = fmt.Fprintf(w, `{"data":[%s],"meta":{"pagination":{"page":%d,"pageSize":1,"total":100000,"totalPages":100000}}}`, oneMemberJSON, page)
	})

	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{PageSize: 1})

	if err == nil {
		t.Fatal("a sweep over 100000 pages returned without an error")
	}
	apiErr := asAPIError(t, err)
	if apiErr.Code != "PAGE_LIMIT_EXCEEDED" {
		t.Errorf("Code = %q, want PAGE_LIMIT_EXCEEDED (err: %v)", apiErr.Code, err)
	}
	if got := requests.Load(); got != 1000 {
		t.Errorf("%d requests were made, want exactly the cap of 1000", got)
	}
	if page == nil || len(page.Members) != 1000 {
		n := -1
		if page != nil {
			n = len(page.Members)
		}
		t.Fatalf("partial result has %d members, want the 1000 fetched before the cap", n)
	}
	if page.Pagination.Page != 1000 {
		t.Errorf("Pagination.Page = %d, want the last page fetched (1000)", page.Pagination.Page)
	}
	if apiErr.RequestID == "" {
		t.Error("RequestID is empty")
	}
	if got := ExitCodeFor(err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
}

// Exactly the cap's worth of pages is a complete answer, not a truncated one.
func TestListAllMembersAtExactlyTheCapIsComplete(t *testing.T) {
	var seen []int
	c := pagesServer(t, 1000, 1, 0, 0, &seen)

	page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{PageSize: 1})
	if err != nil {
		t.Fatalf("ListAllMembers over exactly 1000 pages: %v", err)
	}
	if len(page.Members) != 1000 {
		t.Errorf("members = %d, want 1000", len(page.Members))
	}
}

// Without meta.pagination the loop cannot tell where the last page is. Stopping
// silently would pass a prefix off as the whole list.
func TestListAllMembersFailsWhenThePaginationIsMissing(t *testing.T) {
	bodies := map[string]string{
		"no meta":             `{"data":[` + oneMemberJSON + `]}`,
		"empty meta":          `{"data":[` + oneMemberJSON + `],"meta":{}}`,
		"null meta":           `{"data":[` + oneMemberJSON + `],"meta":null}`,
		"null pagination":     `{"data":[` + oneMemberJSON + `],"meta":{"pagination":null}}`,
		"only unrelated keys": `{"data":[` + oneMemberJSON + `],"meta":{"other":1}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(body))
			})

			page, err := c.ListAllMembers(context.Background(), "acme", ListMembersParams{})

			if err == nil {
				t.Fatal("--all stopped silently on a response with no pagination")
			}
			apiErr := asAPIError(t, err)
			if !strings.Contains(apiErr.Message, "pagination") {
				t.Errorf("message does not say what is missing: %q", apiErr.Message)
			}
			if apiErr.RequestID == "" || string(apiErr.RawBody) != body {
				t.Errorf("request id %q / raw body %q", apiErr.RequestID, apiErr.RawBody)
			}
			if page == nil || len(page.Members) != 1 {
				t.Errorf("partial result = %+v, want the page that did arrive", page)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("%d requests, want 1: without pagination there is no next page to ask for", got)
			}
			if got := ExitCodeFor(err); got == 0 {
				t.Error("exit code 0 for a failed sweep")
			}
		})
	}
}

// A single page is not a sweep: with no pagination in the reply, list prints the
// rows it got, and the meta the API sent, as it always did.
func TestListMembersWithoutPaginationIsNotAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[` + oneMemberJSON + `]}`))
	})
	page, err := c.ListMembers(context.Background(), "acme", ListMembersParams{})
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(page.Members) != 1 || len(page.Meta) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestListAllMembersStopsWhenTheContextIsCancelled(t *testing.T) {
	var seen []int
	srv := pagesServer(t, 50, 1, 0, 0, &seen)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := srv.ListAllMembers(ctx, "acme", ListMembersParams{}); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if len(seen) != 0 {
		t.Errorf("a cancelled context still made %d requests", len(seen))
	}
}

const memberBody = `{"data":{
	"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	"userId": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
	"level": "owner",
	"status": "active",
	"createdAt": "2026-07-24T09:30:00.000Z",
	"displayName": null,
	"avatarUrl": null,
	"contactEmail": null,
	"contactPhone": null,
	"jobTitle": null,
	"department": null,
	"memberCode": null
}}`

func TestGetMemberSuccess(t *testing.T) {
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(memberBody))
	})

	member, err := c.GetMember(context.Background(), "acme", "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if gotPath != "/api/members/a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("path: got %q", gotPath)
	}
	if member.Level != "owner" {
		t.Errorf("Level: got %q, want %q", member.Level, "owner")
	}
	if member.DisplayName != nil {
		t.Errorf("DisplayName: got %v, want nil", member.DisplayName)
	}
}

// The id comes from the command line. Whatever it holds stays one path segment.
func TestGetMemberEscapesTheID(t *testing.T) {
	var gotEscaped string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotEscaped = r.URL.EscapedPath()
		_, _ = w.Write([]byte(memberBody))
	})

	if _, err := c.GetMember(context.Background(), "acme", "../health?x=1"); err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if gotEscaped != "/api/members/..%2Fhealth%3Fx=1" {
		t.Errorf("escaped path = %q: the id leaked out of its path segment", gotEscaped)
	}
}

func TestGetMemberNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"E0404_NOT_FOUND","message":"not found"}}`))
	})

	_, err := c.GetMember(context.Background(), "acme", "00000000-0000-0000-0000-000000000000")
	apiErr := asAPIError(t, err)
	if apiErr.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusNotFound)
	}
	if got := ExitCodeFor(err); got != 4 {
		t.Errorf("exit code = %d, want 4", got)
	}
}

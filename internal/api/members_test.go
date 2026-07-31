package api

import (
	"context"
	"net/http"
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
      "displayName": "Nguyen Van A",
      "avatarUrl": null,
      "contactEmail": "a.nguyen@example.com",
      "contactPhone": null,
      "jobTitle": "Software Engineer",
      "department": "Engineering",
      "memberCode": "M-001"
    }
  ],
  "meta": { "pagination": { "page": 1, "pageSize": 20, "total": 1, "totalPages": 1 } }
}`

func TestListMembersSuccess(t *testing.T) {
	var gotPage, gotPageSize, gotStatus, gotTenant string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/members" {
			t.Errorf("path: got %q, want %q", r.URL.Path, "/members")
		}
		gotPage = r.URL.Query().Get("page")
		gotPageSize = r.URL.Query().Get("pageSize")
		gotStatus = r.URL.Query().Get("status")
		gotTenant = r.Header.Get("X-Tenant-Code")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(membersListBody))
	})

	members, meta, err := c.ListMembers(context.Background(), "acme", ListMembersParams{Page: 2, PageSize: 20, Status: "active"})
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if gotPage != "2" {
		t.Errorf("page query: got %q, want %q", gotPage, "2")
	}
	if gotPageSize != "20" {
		t.Errorf("pageSize query: got %q, want %q", gotPageSize, "20")
	}
	if gotStatus != "active" {
		t.Errorf("status query: got %q, want %q", gotStatus, "active")
	}
	if gotTenant != "acme" {
		t.Errorf("tenant header: got %q, want %q", gotTenant, "acme")
	}
	if len(members) != 1 {
		t.Fatalf("members length: got %d, want 1", len(members))
	}
	if members[0].ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("member ID: got %q", members[0].ID)
	}
	if members[0].AvatarURL != nil {
		t.Errorf("AvatarURL: got %v, want nil", members[0].AvatarURL)
	}
	if members[0].ContactEmail == nil || *members[0].ContactEmail != "a.nguyen@example.com" {
		t.Errorf("ContactEmail: got %v", members[0].ContactEmail)
	}
	if meta.Pagination.TotalPages != 1 {
		t.Errorf("TotalPages: got %d, want 1", meta.Pagination.TotalPages)
	}
}

func TestListMembersOmitsUnsetQueryParams(t *testing.T) {
	var query map[string][]string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(membersListBody))
	})

	if _, _, err := c.ListMembers(context.Background(), "acme", ListMembersParams{}); err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if _, ok := query["page"]; ok {
		t.Error("page query param should be omitted when unset")
	}
	if _, ok := query["status"]; ok {
		t.Error("status query param should be omitted when unset")
	}
}

func TestGetMemberSuccess(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/members/a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{
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
		}}`))
	})

	member, err := c.GetMember(context.Background(), "acme", "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if member.Level != "owner" {
		t.Errorf("Level: got %q, want %q", member.Level, "owner")
	}
	if member.DisplayName != nil {
		t.Errorf("DisplayName: got %v, want nil", member.DisplayName)
	}
}

func TestGetMemberNotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"E0404_NOT_FOUND","message":"not found"}}`))
	})

	_, err := c.GetMember(context.Background(), "acme", "00000000-0000-0000-0000-000000000000")
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus: got %d, want %d", apiErr.HTTPStatus, http.StatusNotFound)
	}
}

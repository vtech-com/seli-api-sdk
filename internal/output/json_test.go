package output

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// A caller distinguishes "no rows" from "no such key". A nil slice marshals to
// null, which reads as neither.
func TestWriteEmitsAnEmptyArrayForNoRows(t *testing.T) {
	var buf bytes.Buffer
	var none []member
	if err := Write(&buf, none, Meta{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(buf.String(), `"data": []`) {
		t.Errorf("nil slice must emit [], got:\n%s", buf.String())
	}
}

// A single item is an object at .data, not a one-element array, and not the
// top level.
func TestWritePutsASingleItemAtData(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, member{ID: "m1", Name: "A"}, Meta{Tenant: "acme", TenantSource: "flag"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got struct {
		Data member            `json:"data"`
		Meta map[string]any    `json:"meta"`
		Err  map[string]string `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data.ID != "m1" || got.Meta["tenant"] != "acme" || got.Meta["tenant_source"] != "flag" {
		t.Errorf("got %+v", got)
	}
	if got.Err != nil {
		t.Errorf("a successful write carries an error key: %v", got.Err)
	}
}

// A command that is not tenant-scoped must not claim a tenant it never resolved.
func TestWriteOmitsTenantWhenThereIsNone(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, member{ID: "m1"}, Meta{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(buf.String(), "tenant") {
		t.Errorf("empty meta must not mention a tenant:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"meta": {}`) {
		t.Errorf("empty meta must be {}:\n%s", buf.String())
	}
}

func TestMetaCarriesTheAPIsOwnKeysVerbatim(t *testing.T) {
	meta := Meta{
		Tenant:       "acme",
		TenantSource: "env",
		Extra: map[string]any{
			"pagination": json.RawMessage(`{"page":2,"pageSize":20,"total":41,"totalPages":3}`),
			"future":     json.RawMessage(`[1,2,3]`),
		},
	}
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Known keys first, in a fixed order, then the API's keys; the API's own
	// field names and order inside each value are untouched.
	const want = `{"tenant":"acme","tenant_source":"env","future":[1,2,3],"pagination":{"page":2,"pageSize":20,"total":41,"totalPages":3}}`
	if string(b) != want {
		t.Errorf("meta =\n%s\nwant\n%s", b, want)
	}
}

func TestMetaExtraAloneAndNeither(t *testing.T) {
	b, err := json.Marshal(Meta{Extra: map[string]any{"pagination": json.RawMessage(`{"page":1}`)}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `{"pagination":{"page":1}}` {
		t.Errorf("extra-only meta = %s", b)
	}
	b, err = json.Marshal(Meta{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `{}` {
		t.Errorf("empty meta = %s", b)
	}
}

// An API reply passed through as raw JSON must survive normalizeData: it is a
// []byte underneath, and the nil-slice rule must not rewrite it.
func TestWritePassesRawMessagesThroughUntouched(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, json.RawMessage(`{"ok":true}`), Meta{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got struct {
		Data map[string]bool `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil || !got.Data["ok"] {
		t.Errorf("raw data lost: %v\n%s", err, buf.String())
	}

	buf.Reset()
	if err := Write(&buf, json.RawMessage(nil), Meta{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(buf.String(), `"data": null`) {
		t.Errorf("an empty raw message must be null, not an invalid document:\n%s", buf.String())
	}
}

// stdout stays parseable when the command fails: a caller that reads stdout
// unconditionally gets a diagnosis, not a parse error on top of an API error.
func TestRenderErrorPutsJSONOnStdoutAndASummaryOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	RenderError(&stdout, &stderr, ErrorDetail{
		Code: "E0003_UNAUTHORIZED", Message: "Unauthorized",
		RequestID: "req_7", HTTPStatus: 401, RawBody: `{"error":{"code":"E0003_UNAUTHORIZED"}}`,
		Next: "do the thing", CapabilityNote: true,
	})

	var got struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got.Error["code"] != "E0003_UNAUTHORIZED" || got.Error["request_id"] != "req_7" {
		t.Errorf("error object lost fields: %v", got.Error)
	}
	if got.Error["http_status"] != float64(401) {
		t.Errorf("http_status = %v, want 401", got.Error["http_status"])
	}
	if got.Error["raw"] != `{"error":{"code":"E0003_UNAUTHORIZED"}}` {
		t.Errorf("raw = %v, want the verbatim server body", got.Error["raw"])
	}
	if got.Error["next"] != "do the thing" {
		t.Errorf("next = %v", got.Error["next"])
	}
	// The brake is the sentence, not a boolean.
	note, _ := got.Error["capability_note"].(string)
	if !strings.Contains(note, "does NOT mean this operation is unsupported") {
		t.Errorf("capability_note is not the brake sentence: %v", got.Error["capability_note"])
	}
	if !strings.Contains(stderr.String(), "Error: Unauthorized") || !strings.Contains(stderr.String(), "request_id=req_7") {
		t.Errorf("stderr summary missing:\n%s", stderr.String())
	}
}

// Without a value, a key is absent rather than empty or false: a caller scanning
// for it should not find one that says nothing.
func TestRenderErrorOmitsWhatDoesNotApply(t *testing.T) {
	var stdout, stderr bytes.Buffer
	RenderError(&stdout, &stderr, ErrorDetail{Code: "VALIDATION_ERROR", Message: "--page must be positive"})

	var got struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	for _, key := range []string{"capability_note", "next", "meaning", "raw", "request_id", "http_status"} {
		if _, present := got.Error[key]; present {
			t.Errorf("key %q should be absent, got %v", key, got.Error[key])
		}
	}
	if got.Error["code"] != "VALIDATION_ERROR" || got.Error["message"] != "--page must be positive" {
		t.Errorf("error = %v", got.Error)
	}
}

// A locally-diagnosed error never left the machine and has no request id to
// quote; printing "request_id=" with nothing after it would suggest one was lost.
func TestRenderErrorSummaryNamesTheRequestIDOnlyWhenThereIsOne(t *testing.T) {
	var with, without bytes.Buffer
	RenderErrorSummary(&with, ErrorDetail{Code: "C", Message: "m", RequestID: "abc"})
	RenderErrorSummary(&without, ErrorDetail{Code: "C", Message: "m"})

	if with.String() != "Error: m (code=C, request_id=abc)\n" {
		t.Errorf("with id: %q", with.String())
	}
	if without.String() != "Error: m (code=C)\n" {
		t.Errorf("without id: %q", without.String())
	}
}

// The failure and the rows fetched before it travel in one document: the error
// key above the data, so a caller that tests for the key knows it holds a prefix
// of the truth. stdout must hold exactly that one document.
func TestWritePartialIsOneDocumentWithTheErrorAboveTheRows(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rows := []member{{ID: "m1"}, {ID: "m2"}}
	meta := Meta{Tenant: "acme", TenantSource: "flag", Extra: map[string]any{"pagination": json.RawMessage(`{"page":2}`)}}
	err := WritePartial(&stdout, &stderr, rows, meta, ErrorDetail{
		Code: "E0000_UNKNOWN", Message: "boom", RequestID: "req_9", HTTPStatus: 500,
	})
	if err != nil {
		t.Fatalf("WritePartial: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var doc struct {
		Error map[string]any `json:"error"`
		Data  []member       `json:"data"`
		Meta  map[string]any `json:"meta"`
	}
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Errorf("stdout holds more than one JSON document (next token err = %v):\n%s", err, stdout.String())
	}

	if doc.Error["code"] != "E0000_UNKNOWN" || doc.Error["request_id"] != "req_9" {
		t.Errorf("error = %v", doc.Error)
	}
	if len(doc.Data) != 2 || doc.Data[1].ID != "m2" {
		t.Errorf("data = %+v, want the rows fetched before the failure", doc.Data)
	}
	if doc.Meta["tenant"] != "acme" || doc.Meta["pagination"] == nil {
		t.Errorf("meta = %v", doc.Meta)
	}
	if strings.Index(stdout.String(), `"error"`) > strings.Index(stdout.String(), `"data"`) {
		t.Errorf("the error key must come before the rows it qualifies:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Error: boom") {
		t.Errorf("stderr summary missing:\n%s", stderr.String())
	}
}

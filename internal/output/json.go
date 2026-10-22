// Package output writes what a command has to say, as JSON, to stdout.
//
// There is one output shape and no way to ask for another. Every successful
// command emits the same envelope:
//
//	{ "data": …, "meta": { … } }
//
// data is an array for a list and an object for a single item; meta names the
// tenant the call resolved to, and carries whatever else the API put in its own
// meta (a list's pagination object, as the API sent it). A caller reads .data
// and .meta and is done — there is no second shape to branch on, and nothing
// true of the call that lives only in prose on a stream nobody parses.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Meta is the envelope's meta object. Every field is optional: a command that
// resolves no tenant, and whose API response carries no meta, prints {}.
type Meta struct {
	// Tenant is the tenant code the call resolved to, and TenantSource says
	// where that value came from: "flag", "env" (SELI_TENANT) or "config"
	// (default_tenant). A command that is not tenant-scoped leaves both empty.
	Tenant       string `json:"tenant,omitempty"`
	TenantSource string `json:"tenant_source,omitempty"`

	// Extra is whatever the API put in its own meta, passed through untouched.
	// GET /api/members sends `pagination: {page, pageSize, total, totalPages}`;
	// the next key an endpoint adds should not need a release here.
	//
	// The CLI does not model the API's responses. That rule applies to `meta`
	// exactly as it does to `data`: renaming the API's fields into a shape of
	// our own is how a key gets silently dropped while every check says "match".
	Extra map[string]any `json:"-"`
}

// MarshalJSON writes the known keys in a fixed order, then whatever else the API
// sent. Ordering is not semantics, but a caller reading a page and a response
// side by side should not have to hunt.
func (m Meta) MarshalJSON() ([]byte, error) {
	type meta Meta // shed this method, keep the tags
	known, err := json.Marshal(meta(m))
	if err != nil {
		return nil, err
	}
	if len(m.Extra) == 0 {
		return known, nil
	}
	extra, err := json.Marshal(m.Extra)
	if err != nil {
		return nil, err
	}
	if string(known) == "{}" {
		return extra, nil
	}
	// `{a}` + `{b}` -> `{a,b}`
	out := append(known[:len(known)-1], ',')
	return append(out, extra[1:]...), nil
}

// envelope is the one shape this CLI emits.
//
// Error comes first so that a reader — human or machine — meets it before the
// rows it qualifies. It is absent on success, and its presence is the whole
// test: a document with an error key is not a complete answer, whatever the
// rows and the meta beside it may look like.
type envelope struct {
	Error any  `json:"error,omitempty"`
	Data  any  `json:"data"`
	Meta  Meta `json:"meta"`
}

// Write emits the envelope to w. A nil or nil-slice data becomes [], so an
// empty result is never indistinguishable from a missing one.
func Write(w io.Writer, data any, meta Meta) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope{Data: normalizeData(data), Meta: meta})
}

// WritePartial emits one document carrying both the failure and the rows that
// were fetched before it: an --all sweep that aborted on page 41 still holds
// forty real pages.
//
// Neither throwing the rows away nor printing them under a clean envelope is
// honest. Thrown away, a caller must refetch what the CLI already has. Printed
// alone, they are indistinguishable from a complete answer — a members list
// that stopped at page 41 of 90 reports a clean page of rows, which is exactly
// what success looks like.
//
// So the rows stand, and the error key stands over them. A caller tests for the
// key; it does not have to reach for the exit code, or read stderr, to know it
// is holding a prefix of the truth.
func WritePartial(stdout, stderr io.Writer, data any, meta Meta, d ErrorDetail) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	err := enc.Encode(envelope{Error: errorObject(d), Data: normalizeData(data), Meta: meta})
	RenderErrorSummary(stderr, d)
	return err
}

// normalizeData turns a nil slice into an empty one. A nil slice marshals to
// null, which reads as neither "no rows" nor "no such key".
func normalizeData(data any) any {
	// A raw message is the API's own bytes, passed through untouched. It is a
	// []byte underneath, so the nil-slice rule below would rewrite it into an
	// empty one, which is not valid JSON. An empty raw message is null.
	if rm, ok := data.(json.RawMessage); ok {
		if len(rm) == 0 {
			return json.RawMessage("null")
		}
		return rm
	}
	rv := reflect.ValueOf(data)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return data
}

const capabilityBrake = "a failed write does NOT mean this operation is unsupported."

// ErrorDetail carries everything needed to render a self-diagnosing error: the
// raw API fields plus the catalog interpretation (meaning, next step, and the
// capability brake). Meaning/Next/RawBody may be empty when no catalog entry
// matched.
type ErrorDetail struct {
	Code           string
	Message        string
	RequestID      string
	HTTPStatus     int
	Meaning        string
	Next           string
	CapabilityNote bool
	RawBody        string
}

// RenderError writes a failure as JSON on stdout and a one-line summary on
// stderr. stdout stays machine-readable even when the command failed: a caller
// that parses stdout unconditionally gets an object with an "error" key rather
// than a parse error on top of an API error.
func RenderError(stdout, stderr io.Writer, d ErrorDetail) {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"error": errorObject(d)})
	RenderErrorSummary(stderr, d)
}

// errorObject builds the error key. It is shared by RenderError, which prints
// nothing else, and WritePartial, which prints it above the rows it qualifies —
// so a failure reads the same whether or not any data came back with it.
//
// Only the keys that carry something. A `"next": ""` forces a caller to tell an
// absent step apart from an empty one, which is a distinction without a
// difference and a branch nobody writes correctly.
func errorObject(d ErrorDetail) map[string]any {
	e := map[string]any{"code": d.Code, "message": d.Message}
	putIf := func(k string, v string) {
		if v != "" {
			e[k] = v
		}
	}
	putIf("request_id", d.RequestID)
	putIf("meaning", d.Meaning)
	putIf("next", d.Next)
	putIf("raw", d.RawBody)
	if d.HTTPStatus > 0 {
		e["http_status"] = d.HTTPStatus
	}
	// The brake is the sentence, not a boolean: `"capability_note": true` tells
	// a reader nothing, while the sentence is the thing that stops an agent
	// concluding "the API cannot do this" from a request it got wrong.
	if d.CapabilityNote {
		e["capability_note"] = capabilityBrake
	}
	return e
}

// RenderErrorSummary writes only the stderr line.
//
// Use it when the command has already written its envelope to stdout — a
// partial --all sweep, say. stdout must hold exactly one JSON document: a
// caller running json.load on an envelope followed by an error object gets
// "Extra data", which is the very failure the single-shape contract exists to
// remove. When data has been printed, the failure is carried by the exit code
// and by this line, and nothing more is added to stdout.
//
// The request id is named only when there is one: a locally-diagnosed error
// never left the machine and has none to quote.
func RenderErrorSummary(stderr io.Writer, d ErrorDetail) {
	if d.RequestID == "" {
		_, _ = fmt.Fprintf(stderr, "Error: %s (code=%s)\n", d.Message, d.Code)
		return
	}
	_, _ = fmt.Fprintf(stderr, "Error: %s (code=%s, request_id=%s)\n", d.Message, d.Code, d.RequestID)
}

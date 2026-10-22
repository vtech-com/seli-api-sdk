package cmd

import (
	"encoding/json"

	"github.com/vtech-com/seli-api-sdk/internal/output"
)

// The tenant a command resolved to is the one fact about a call that the API's
// own response never carries. A caller that has just read the wrong tenant's
// members needs to be able to read that fact, not notice it. It goes in meta,
// next to the layer it was resolved from ("flag", "env" or "config"), so a
// silently defaulted tenant is visible rather than assumed.

// itemMeta is the meta of a single-record response: a get, or a health check.
// tenant and source come from resolveTenant.
func itemMeta(tenant, source string) output.Meta {
	return output.Meta{Tenant: tenant, TenantSource: source}
}

// listMeta is the meta of a list response: the tenant fields, plus the API's own
// meta passed through untouched. For GET /api/members that is the `pagination`
// object, with the keys the API names it by — the CLI does not rename them.
// raw may be absent.
func listMeta(tenant, source string, raw json.RawMessage) output.Meta {
	m := itemMeta(tenant, source)
	m.Extra = apiMeta(raw)
	return m
}

// apiMeta lifts the API's meta object into Extra, one raw value per key, so
// every key survives byte for byte whatever it holds. An absent, null, or
// non-object meta yields nil: no meta is not the same answer as a meta of
// zeros, and the CLI invents neither.
func apiMeta(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) == 0 {
		return nil
	}
	extra := make(map[string]any, len(fields))
	for k, v := range fields {
		extra[k] = v
	}
	return extra
}

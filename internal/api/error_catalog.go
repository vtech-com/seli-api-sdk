package api

// ErrorInfo is a human- and agent-actionable interpretation of a Seli error
// code. The server already returns a descriptive `message`; this catalog adds
// only what that message lacks:
//
//   - Next: the concrete action to take (never part of the server message).
//   - CapabilityNote: the "a failed write is not a missing capability" brake.
//   - Meaning: an interpretation, for codes whose server message is a generic
//     fallback. For every other code the server message is descriptive enough,
//     so Meaning is left empty to avoid maintaining a second, drift-prone copy
//     of it.
type ErrorInfo struct {
	Meaning        string
	Next           string
	CapabilityNote bool
}

// LookupError returns the catalog entry for a code, if known.
func LookupError(code string) (ErrorInfo, bool) {
	info, ok := errorCatalog[code]
	return info, ok
}

// errorCatalog is deliberately empty. It is wired end to end — LookupError,
// the renderer's enrichment, the `next` / `meaning` / `capability_note` keys —
// so that adding an entry is the only step a real code ever needs.
//
// An entry belongs here only when it is taken from the Seli backend's own
// error code list, with that list as the source. Never guess a code, a
// meaning, or a next step from what a response happened to look like once, and
// never copy another product's codes: a wrong interpretation is worse than
// none, because an agent will act on it.
var errorCatalog = map[string]ErrorInfo{}

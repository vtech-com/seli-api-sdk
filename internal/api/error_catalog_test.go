package api

import "testing"

func TestLookupError_UnknownCodeIsNotFound(t *testing.T) {
	for _, code := range []string{"", "E0003_UNAUTHORIZED", "AUTH_MISSING_HEADER", "NOT_A_CODE"} {
		if info, ok := LookupError(code); ok {
			t.Errorf("LookupError(%q) = %+v, want no entry: entries come only from the backend's error code list", code, info)
		}
	}
}

// The catalog ships empty but wired: adding one entry is all a real code needs.
func TestLookupError_ReturnsAnEntryOnceThereIsOne(t *testing.T) {
	const code = "TEST_ONLY_CODE"
	errorCatalog[code] = ErrorInfo{Meaning: "m", Next: "n", CapabilityNote: true}
	t.Cleanup(func() { delete(errorCatalog, code) })

	info, ok := LookupError(code)
	if !ok {
		t.Fatal("LookupError does not see an entry added to the catalog")
	}
	if info.Meaning != "m" || info.Next != "n" || !info.CapabilityNote {
		t.Errorf("entry = %+v", info)
	}
}

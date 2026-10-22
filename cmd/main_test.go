package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/vtech-com/seli-api-sdk/internal/version"
)

// The CLI ends a failed command with os.Exit, which an in-process test cannot
// observe. So the end-to-end tests run the real command tree in a child
// process: the test binary re-executes itself, and TestMain — seeing the
// environment variable below — runs Execute() instead of the tests. The parent
// gets the exit code, stdout and stderr exactly as a shell would.
const (
	childEnv        = "SELI_TEST_AS_CLI"
	childArgsEnv    = "SELI_TEST_ARGS"
	childDefaultEnv = "SELI_TEST_DEFAULT_API_URL"
)

func TestMain(m *testing.M) {
	if os.Getenv(childEnv) == "1" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv(childArgsEnv)), &args); err != nil {
			os.Exit(99)
		}
		os.Args = append([]string{"seli"}, args...)
		// Stands in for the -ldflags -X that real builds apply.
		version.DefaultAPIURL = os.Getenv(childDefaultEnv)
		Execute()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliResult struct {
	stdout, stderr string
	exit           int
}

// cliRun runs `seli <args>` in a child process with exactly the environment
// given plus a private HOME — never the developer's own SELI_* variables or
// ~/.seli config. home may be "" for a fresh empty one.
func cliRun(t *testing.T, home string, env map[string]string, args ...string) cliResult {
	t.Helper()
	if home == "" {
		home = t.TempDir()
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	cmd := exec.Command(exe)
	cmd.Env = []string{
		childEnv + "=1",
		childArgsEnv + "=" + string(argsJSON),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	res := cliResult{}
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		res.exit = ee.ExitCode()
	}
	res.stdout, res.stderr = stdout.String(), stderr.String()
	return res
}

// doc decodes stdout, which must be exactly one JSON document.
func (r cliResult) doc(t *testing.T) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(r.stdout)))
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout is not a JSON document: %v\nstdout: %q\nstderr: %q", err, r.stdout, r.stderr)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout holds more than one JSON document:\n%s", r.stdout)
	}
	return out
}

// obj digs out doc[k1][k2]... as a JSON object.
func obj(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			t.Fatalf("no object at %q in %v", k, cur)
		}
		cur = next
	}
	return cur
}

// recordedRequest is what the fake API saw.
type recordedRequest struct {
	method, path, rawQuery string
	header                 http.Header
}

// fakeAPI is an httptest server that records every request before answering it.
// It listens on 127.0.0.1, the one plain-HTTP host the client accepts.
type fakeAPI struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []recordedRequest
}

func newFakeAPI(t *testing.T, handler http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reqs = append(f.reqs, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()})
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) url() string { return f.srv.URL }

func (f *fakeAPI) requests() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.reqs...)
}

// writeConfigFile puts a config.json under home/.seli, where the CLI looks.
func writeConfigFile(t *testing.T, home, contents string) string {
	t.Helper()
	dir := filepath.Join(home, ".seli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const (
	healthBody = `{"data":{"ok":true,"timestamp":"2026-07-31T09:30:00.000Z"}}`
	testAPIKey = "ssk_0123456789abcdefSECRET"
)

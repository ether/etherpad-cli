// Copyright 2026 john-mclear. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// captureServer spins up an httptest server that records the path and JSON
// body of the request it receives and replies with a benign success envelope.
// It returns the server plus pointers the test reads after invoking a command.
func captureServer(t *testing.T) (*httptest.Server, *string, *map[string]any) {
	t.Helper()
	var gotPath string
	gotBody := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = map[string]any{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath, &gotBody
}

// runPromoted executes a promoted command against the capture server in a fully
// isolated environment (temp HOME/config, no real credentials).
func runPromoted(t *testing.T, baseURL string, args ...string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ETHERPAD_CONFIG", filepath.Join(home, "config.toml")) // nonexistent -> defaults
	t.Setenv("ETHERPAD_OPENID", "")
	t.Setenv("ETHERPAD_BASE_URL", baseURL)

	root := RootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append(args, "--agent"))
	if err := root.Execute(); err != nil {
		t.Fatalf("command %v failed: %v", args, err)
	}
}

// TestPromotedSerializesFlagsIntoBody is the regression test for issue #1:
// every parameterized command must serialize its bound flags into the POST
// body. Before the fix the body was always {} and the server rejected the call.
func TestPromotedSerializesFlagsIntoBody(t *testing.T) {
	cases := []struct {
		name string
		args []string
		path string
		want map[string]any
	}{
		{
			name: "get-text",
			args: []string{"get-text", "--pad-id", "mypad", "--rev", "3"},
			path: "/getText",
			want: map[string]any{"padID": "mypad", "rev": "3"},
		},
		{
			// set-text exercises the lower-case-d "authorId" spelling that the
			// Etherpad spec uses for this endpoint (distinct from "authorID").
			name: "set-text",
			args: []string{"set-text", "--pad-id", "mypad", "--text", "hello", "--author-id", "a.1"},
			path: "/setText",
			want: map[string]any{"padID": "mypad", "text": "hello", "authorId": "a.1"},
		},
		{
			name: "copy-pad",
			args: []string{"copy-pad", "--source-id", "src", "--destination-id", "dst", "--force", "true"},
			path: "/copyPad",
			want: map[string]any{"sourceID": "src", "destinationID": "dst", "force": "true"},
		},
		{
			// anonymize-author exercises the upper-case "authorID" spelling.
			name: "anonymize-author",
			args: []string{"anonymize-author", "--author-id", "a.42"},
			path: "/anonymizeAuthor",
			want: map[string]any{"authorID": "a.42"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, gotPath, gotBody := captureServer(t)
			runPromoted(t, srv.URL, tc.args...)

			if *gotPath != tc.path {
				t.Errorf("path: want %s, got %s", tc.path, *gotPath)
			}
			for k, v := range tc.want {
				got, ok := (*gotBody)[k]
				if !ok {
					t.Errorf("body missing key %q (got body %v)", k, *gotBody)
					continue
				}
				if got != v {
					t.Errorf("body[%q]: want %v, got %v", k, v, got)
				}
			}
		})
	}
}

// TestPromotedOmitsEmptyFlags confirms unset flags are not serialized, so
// optional parameters (e.g. create-pad without a padID) don't send empty
// strings the server would reject.
func TestPromotedOmitsEmptyFlags(t *testing.T) {
	srv, gotPath, gotBody := captureServer(t)
	runPromoted(t, srv.URL, "get-text", "--pad-id", "mypad")

	if *gotPath != "/getText" {
		t.Fatalf("path: want /getText, got %s", *gotPath)
	}
	if _, ok := (*gotBody)["rev"]; ok {
		t.Errorf("unset --rev should be omitted from body, got %v", *gotBody)
	}
	if (*gotBody)["padID"] != "mypad" {
		t.Errorf("body[padID]: want mypad, got %v", (*gotBody)["padID"])
	}
}

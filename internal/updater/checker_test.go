package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckerReportsNewStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/latest" {
				t.Errorf("request path = %q", request.URL.Path)
			}
			if request.Header.Get("Accept") != "application/vnd.github+json" {
				t.Errorf("Accept header = %q", request.Header.Get("Accept"))
			}
			if request.Header.Get("User-Agent") != "RollingThunder/1.4.2" {
				t.Errorf("User-Agent header = %q", request.Header.Get("User-Agent"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{
				"tag_name": "v1.5.0",
				"name": "Rolling Thunder 1.5",
				"body": "Faster exports and safer updates.",
				"draft": false,
				"prerelease": false,
				"published_at": "2026-07-25T01:02:03Z"
			}`)
		},
	))
	defer server.Close()

	checker := newChecker("1.4.2", server.URL+"/latest", server.Client())
	result, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Available {
		t.Fatal("newer release was not reported")
	}
	if result.CurrentVersion != "1.4.2" || result.LatestVersion != "1.5.0" {
		t.Fatalf("versions = current %q latest %q", result.CurrentVersion, result.LatestVersion)
	}
	if result.ReleaseURL != "https://github.com/yudhasubki/rollingthunder/releases/tag/v1.5.0" {
		t.Fatalf("release URL = %q", result.ReleaseURL)
	}
	if result.PublishedAt != "2026-07-25T01:02:03Z" {
		t.Fatalf("publishedAt = %q", result.PublishedAt)
	}
}

func TestCheckerDoesNotOfferOlderOrPrereleaseBuilds(t *testing.T) {
	tests := []struct {
		name    string
		current string
		release string
	}{
		{name: "same version", current: "2.0.0", release: "v2.0.0"},
		{name: "older version", current: "2.1.0", release: "v2.0.9"},
		{name: "current stable beats prerelease", current: "2.0.0", release: "v2.0.0-rc.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(writer http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(
						writer,
						`{"tag_name":%q,"draft":false,"prerelease":false}`,
						test.release,
					)
				},
			))
			defer server.Close()

			result, err := newChecker(
				test.current,
				server.URL,
				server.Client(),
			).Check(context.Background())
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if result.Available {
				t.Fatalf("%s unexpectedly offered %s", test.current, test.release)
			}
		})
	}
}

func TestCheckerTreatsMissingReleaseAsNoUpdate(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	result, err := newChecker("0.1.0", server.URL, server.Client()).Check(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Available || result.LatestVersion != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckerRejectsInvalidOrOversizedResponses(t *testing.T) {
	t.Run("invalid latest version", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(
			func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, `{"tag_name":"latest"}`)
			},
		))
		defer server.Close()

		_, err := newChecker("0.1.0", server.URL, server.Client()).Check(
			context.Background(),
		)
		if err == nil || !strings.Contains(err.Error(), "latest release version") {
			t.Fatalf("Check() error = %v", err)
		}
	})

	t.Run("oversized response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(
			func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(
					writer,
					strings.Repeat("x", maxReleaseResponseSize+1),
				)
			},
		))
		defer server.Close()

		_, err := newChecker("0.1.0", server.URL, server.Client()).Check(
			context.Background(),
		)
		if err == nil || !strings.Contains(err.Error(), "size limit") {
			t.Fatalf("Check() error = %v", err)
		}
	})
}

func TestGitHubClientRejectsUntrustedRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(
				writer,
				request,
				"http://example.com/releases/latest",
				http.StatusFound,
			)
		},
	))
	defer server.Close()

	_, err := newGitHubHTTPClient().Get(server.URL)
	if err == nil || !strings.Contains(err.Error(), "untrusted host") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestSemanticVersionComparison(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "v1.10.0", right: "1.9.9", want: 1},
		{left: "1.0.0", right: "1.0.0", want: 0},
		{left: "1.0.0-rc.2", right: "1.0.0-rc.10", want: -1},
		{left: "1.0.0", right: "1.0.0-rc.1", want: 1},
	}
	for _, test := range tests {
		left, err := parseSemanticVersion(test.left)
		if err != nil {
			t.Fatalf("parse left %q: %v", test.left, err)
		}
		right, err := parseSemanticVersion(test.right)
		if err != nil {
			t.Fatalf("parse right %q: %v", test.right, err)
		}
		if got := compareSemanticVersions(left, right); got != test.want {
			t.Fatalf("compare(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

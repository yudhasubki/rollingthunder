package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/application"
)

const (
	latestReleaseEndpoint  = "https://api.github.com/repos/" + application.GitHubRepository + "/releases/latest"
	releasePageBaseURL     = "https://github.com/" + application.GitHubRepository + "/releases/tag/"
	githubAPIVersion       = "2026-03-10"
	maxReleaseResponseSize = 1 << 20
	maxReleaseNotesLength  = 4_000
	updateRequestTimeout   = 6 * time.Second
)

var semanticVersionPattern = regexp.MustCompile(
	`^[vV]?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`,
)

type CheckResult struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	Name           string `json:"name,omitempty"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
}

type Checker struct {
	currentVersion string
	endpoint       string
	client         *http.Client
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

type semanticVersion struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

func NewChecker(currentVersion string) *Checker {
	return newChecker(
		currentVersion,
		latestReleaseEndpoint,
		newGitHubHTTPClient(),
	)
}

func newGitHubHTTPClient() *http.Client {
	return &http.Client{
		Timeout: updateRequestTimeout,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if len(previous) >= 3 {
				return errors.New("too many GitHub release API redirects")
			}
			if request.URL.Scheme != "https" ||
				!strings.EqualFold(request.URL.Hostname(), "api.github.com") {
				return fmt.Errorf(
					"refuse update redirect to untrusted host %q",
					request.URL.Hostname(),
				)
			}
			return nil
		},
	}
}

func newChecker(currentVersion, endpoint string, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{Timeout: updateRequestTimeout}
	}
	return &Checker{
		currentVersion: strings.TrimSpace(currentVersion),
		endpoint:       endpoint,
		client:         client,
	}
}

func (c *Checker) Check(ctx context.Context) (CheckResult, error) {
	result := CheckResult{CurrentVersion: c.currentVersion}
	current, err := parseSemanticVersion(c.currentVersion)
	if err != nil {
		return result, fmt.Errorf("parse current application version: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, updateRequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		c.endpoint,
		nil,
	)
	if err != nil {
		return result, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	request.Header.Set("User-Agent", application.UserAgent(c.currentVersion))

	httpResponse, err := c.client.Do(request)
	if err != nil {
		return result, fmt.Errorf("request latest Rolling Thunder release: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode == http.StatusNotFound {
		return result, nil
	}
	if httpResponse.StatusCode != http.StatusOK {
		return result, fmt.Errorf(
			"GitHub release API returned %s",
			httpResponse.Status,
		)
	}

	body, err := io.ReadAll(io.LimitReader(
		httpResponse.Body,
		maxReleaseResponseSize+1,
	))
	if err != nil {
		return result, fmt.Errorf("read latest release response: %w", err)
	}
	if len(body) > maxReleaseResponseSize {
		return result, errors.New("latest release response exceeded the size limit")
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return result, fmt.Errorf("decode latest release response: %w", err)
	}
	if release.Draft || release.Prerelease {
		return result, nil
	}

	latest, err := parseSemanticVersion(release.TagName)
	if err != nil {
		return result, fmt.Errorf("parse latest release version: %w", err)
	}
	latestVersion := strings.TrimPrefix(
		strings.TrimPrefix(strings.TrimSpace(release.TagName), "v"),
		"V",
	)
	result.LatestVersion = latestVersion
	result.Name = strings.TrimSpace(release.Name)
	if result.Name == "" {
		result.Name = application.Name + " " + latestVersion
	}
	result.ReleaseNotes = truncateUTF8(
		strings.TrimSpace(release.Body),
		maxReleaseNotesLength,
	)
	result.ReleaseURL = releasePageBaseURL + url.PathEscape(
		strings.TrimSpace(release.TagName),
	)
	result.PublishedAt = normalizedPublishedAt(release.PublishedAt)
	result.Available = compareSemanticVersions(latest, current) > 0
	return result, nil
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	matches := semanticVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) == 0 {
		return semanticVersion{}, fmt.Errorf("invalid semantic version %q", value)
	}

	parts := make([]uint64, 3)
	for index, raw := range matches[1:4] {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("invalid numeric version component %q", raw)
		}
		parts[index] = value
	}

	version := semanticVersion{
		major: parts[0],
		minor: parts[1],
		patch: parts[2],
	}
	if matches[4] != "" {
		version.prerelease = strings.Split(matches[4], ".")
		for _, identifier := range version.prerelease {
			if identifier == "" {
				return semanticVersion{}, fmt.Errorf(
					"invalid semantic version %q",
					value,
				)
			}
		}
	}
	return version, nil
}

func compareSemanticVersions(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{
		{left.major, right.major},
		{left.minor, right.minor},
		{left.patch, right.patch},
	} {
		switch {
		case pair[0] > pair[1]:
			return 1
		case pair[0] < pair[1]:
			return -1
		}
	}

	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}

	shared := min(len(left.prerelease), len(right.prerelease))
	for index := range shared {
		comparison := comparePrereleaseIdentifier(
			left.prerelease[index],
			right.prerelease[index],
		)
		if comparison != 0 {
			return comparison
		}
	}
	switch {
	case len(left.prerelease) > len(right.prerelease):
		return 1
	case len(left.prerelease) < len(right.prerelease):
		return -1
	default:
		return 0
	}
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftNumeric := numericIdentifier(left)
	rightNumber, rightNumeric := numericIdentifier(right)
	switch {
	case leftNumeric && rightNumeric:
		switch {
		case leftNumber > rightNumber:
			return 1
		case leftNumber < rightNumber:
			return -1
		default:
			return 0
		}
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func numericIdentifier(value string) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return number, err == nil
}

func truncateUTF8(value string, maximum int) string {
	if utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maximum])) + "…"
}

func normalizedPublishedAt(value string) string {
	publishedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return publishedAt.UTC().Format(time.RFC3339)
}

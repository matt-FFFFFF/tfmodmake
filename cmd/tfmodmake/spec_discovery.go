package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type githubContentsItem struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
}

type githubLocation struct {
	Owner string
	Repo  string
	Ref   string
	Dir   string
}

type deterministicDiscoveryOptions struct {
	PreferPreview bool
	IncludePreview bool
}

func parseGitHubTreeDirURL(raw string) (githubLocation, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return githubLocation{}, fmt.Errorf("parse github url: %w", err)
	}
	if u.Host != "github.com" {
		return githubLocation{}, fmt.Errorf("unsupported host %q", u.Host)
	}
	// Expected: /{owner}/{repo}/tree/{ref}/{dir...}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 5 {
		return githubLocation{}, fmt.Errorf("unexpected github tree url path %q", u.Path)
	}
	owner := parts[0]
	repo := parts[1]
	if parts[2] != "tree" {
		return githubLocation{}, fmt.Errorf("expected /tree/ in github url path %q", u.Path)
	}
	ref := parts[3]
	dir := strings.Join(parts[4:], "/")
	if owner == "" || repo == "" || ref == "" || dir == "" {
		return githubLocation{}, fmt.Errorf("invalid github tree url path %q", u.Path)
	}
	return githubLocation{Owner: owner, Repo: repo, Ref: ref, Dir: dir}, nil
}

func parseRawGitHubFileURL(raw string) (owner, repo, ref, filePath string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", "", false
	}
	if u.Host != "raw.githubusercontent.com" {
		return "", "", "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expected: /{owner}/{repo}/{ref}/{path...}
	if len(parts) < 4 {
		return "", "", "", "", false
	}
	owner = parts[0]
	repo = parts[1]
	ref = parts[2]
	filePath = strings.Join(parts[3:], "/")
	if owner == "" || repo == "" || ref == "" || filePath == "" {
		return "", "", "", "", false
	}
	return owner, repo, ref, filePath, true
}

func listGitHubDirectoryDownloadURLs(client *http.Client, loc githubLocation, includeGlob string, githubToken string) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if includeGlob == "" {
		includeGlob = "*.json"
	}

	items, err := getGitHubDirectoryContents(client, loc, githubToken)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, item := range items {
		if item.Type != "file" {
			continue
		}
		if item.DownloadURL == "" {
			continue
		}
		matched, err := filepath.Match(includeGlob, item.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid include glob %q: %w", includeGlob, err)
		}
		if !matched {
			continue
		}
		urls = append(urls, item.DownloadURL)
	}

	if len(urls) == 0 {
		return nil, errors.New("no matching files found in github directory")
	}

	sort.Strings(urls)

	return urls, nil
}

func getGitHubDirectoryContents(client *http.Client, loc githubLocation, githubToken string) ([]githubContentsItem, error) {
	if client == nil {
		client = http.DefaultClient
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		loc.Owner, loc.Repo, loc.Dir, url.QueryEscape(loc.Ref))

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github contents request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, buildGitHubContentsError(resp.StatusCode, resp.Status, resp.Header, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read github contents response: %w", err)
	}

	var items []githubContentsItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse github contents response: %w", err)
	}

	// Deterministic order.
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func buildGitHubContentsError(statusCode int, status string, headers http.Header, body string) error {
	// When GitHub rate limits unauthenticated requests, it returns a 403 with a JSON body
	// containing "API rate limit exceeded". Provide a clearer action for users.
	if statusCode == http.StatusForbidden {
		bodyLower := strings.ToLower(body)
		if strings.Contains(bodyLower, "rate limit") {
			remaining := headers.Get("X-RateLimit-Remaining")
			reset := headers.Get("X-RateLimit-Reset")
			resetNote := ""
			if reset != "" {
				if unix, err := strconv.ParseInt(reset, 10, 64); err == nil {
					resetNote = fmt.Sprintf(" (resets at %s)", time.Unix(unix, 0).UTC().Format(time.RFC3339))
				}
			}
			if remaining == "" {
				remaining = "unknown"
			}
			return fmt.Errorf(
				"GitHub API rate limit hit while listing spec files (remaining=%s)%s. Set GITHUB_TOKEN or GH_TOKEN to increase limits, then retry. Original error: %s: %s",
				remaining,
				resetNote,
				status,
				body,
			)
		}
	}

	if body != "" {
		return fmt.Errorf("github contents request failed: %s: %s", status, body)
	}
	return fmt.Errorf("github contents request failed: %s", status)
}

func discoverSiblingSpecsFromRawGitHubSpecURL(client *http.Client, specURL string, includeGlob string, githubToken string) ([]string, error) {
	owner, repo, ref, specPath, ok := parseRawGitHubFileURL(specURL)
	if !ok {
		return nil, fmt.Errorf("spec is not a raw.githubusercontent.com URL: %s", specURL)
	}
	dir := path.Dir(specPath)
	if dir == "." || dir == "/" {
		return nil, fmt.Errorf("unable to infer directory from spec path %q", specPath)
	}

	loc := githubLocation{Owner: owner, Repo: repo, Ref: ref, Dir: dir}
	return listGitHubDirectoryDownloadURLs(client, loc, includeGlob, githubToken)
}

func discoverDeterministicSpecSetFromRawGitHubSpecURL(client *http.Client, specURL string, includeGlobs []string, githubToken string, opts deterministicDiscoveryOptions) ([]string, error) {
	owner, repo, ref, specPath, ok := parseRawGitHubFileURL(specURL)
	if !ok {
		return nil, fmt.Errorf("spec is not a raw.githubusercontent.com URL: %s", specURL)
	}
	anchorDir := path.Dir(specPath)
	if anchorDir == "." || anchorDir == "/" {
		return nil, fmt.Errorf("unable to infer directory from spec path %q", specPath)
	}
	loc := githubLocation{Owner: owner, Repo: repo, Ref: ref, Dir: anchorDir}
	return discoverDeterministicSpecSetFromGitHubDir(client, loc, includeGlobs, githubToken, opts)
}

func discoverDeterministicSpecSetFromGitHubDir(client *http.Client, loc githubLocation, includeGlobs []string, githubToken string, opts deterministicDiscoveryOptions) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if len(includeGlobs) == 0 {
		includeGlobs = []string{"*.json"}
	}

	// Try to interpret loc.Dir as either a version folder, a stable/preview folder,
	// or a service root folder containing stable/preview. Choose latest deterministically.
	stableRoot, previewRoot, hasStabilityRoots := inferSiblingStabilityRoots(loc.Dir)

	var wantedRoots []string
	if opts.PreferPreview {
		wantedRoots = append(wantedRoots, previewRoot)
		if opts.IncludePreview {
			// Prefer preview first, then stable.
			wantedRoots = append(wantedRoots, stableRoot)
		}
	} else {
		wantedRoots = append(wantedRoots, stableRoot)
		if opts.IncludePreview {
			wantedRoots = append(wantedRoots, previewRoot)
		}
		// If stable fails (e.g. only preview exists), we still try preview as fallback.
		if !opts.IncludePreview {
			wantedRoots = append(wantedRoots, previewRoot)
		}
	}

	seen := make(map[string]struct{})
	var out []string

	// First: if this directory already contains matching files, use it directly.
	// This keeps behavior intuitive when the anchor is already a version folder.
	if urls, ok := tryListMatchingFilesInDir(client, loc, includeGlobs, githubToken); ok {
		for _, u := range urls {
			if _, exists := seen[u]; exists {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
		return out, nil
	}

	// If we can infer stable/preview sibling roots from the path, jump directly.
	if hasStabilityRoots {
		for _, root := range wantedRoots {
			if root == "" {
				continue
			}
			urls, err := discoverLatestVersionFilesFromStabilityRoot(client, githubLocation{Owner: loc.Owner, Repo: loc.Repo, Ref: loc.Ref, Dir: root}, includeGlobs, githubToken)
			if err != nil {
				continue
			}
			for _, u := range urls {
				if _, exists := seen[u]; exists {
					continue
				}
				seen[u] = struct{}{}
				out = append(out, u)
			}
			if len(out) > 0 && !opts.IncludePreview {
				// Deterministic “good enough” starting point: stop after the first successful root.
				return out, nil
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Otherwise: inspect the directory and decide what it is.
	items, err := getGitHubDirectoryContents(client, loc, githubToken)
	if err != nil {
		return nil, err
	}

	// Service root case: has stable/preview subdirs.
	stableDirItem := findDirItem(items, "stable")
	previewDirItem := findDirItem(items, "preview")
	if stableDirItem != nil || previewDirItem != nil {
		// Compute roots relative to current loc.Dir.
		stableCandidate := strings.TrimSuffix(loc.Dir, "/") + "/stable"
		previewCandidate := strings.TrimSuffix(loc.Dir, "/") + "/preview"
		for _, root := range orderedNonEmptyRoots(stableCandidate, previewCandidate, opts) {
			urls, err := discoverLatestVersionFilesFromStabilityRoot(client, githubLocation{Owner: loc.Owner, Repo: loc.Repo, Ref: loc.Ref, Dir: root}, includeGlobs, githubToken)
			if err != nil {
				continue
			}
			for _, u := range urls {
				if _, exists := seen[u]; exists {
					continue
				}
				seen[u] = struct{}{}
				out = append(out, u)
			}
			if len(out) > 0 && !opts.IncludePreview {
				return out, nil
			}
		}
		// Fallback: if we're in stable-only mode and stable is missing/empty, try preview.
		if len(out) == 0 && !opts.IncludePreview && previewCandidate != "" {
			urls, err := discoverLatestVersionFilesFromStabilityRoot(client, githubLocation{Owner: loc.Owner, Repo: loc.Repo, Ref: loc.Ref, Dir: previewCandidate}, includeGlobs, githubToken)
			if err == nil {
				for _, u := range urls {
					if _, exists := seen[u]; exists {
						continue
					}
					seen[u] = struct{}{}
					out = append(out, u)
				}
				if len(out) > 0 {
					return out, nil
				}
			}
		}
		if len(out) > 0 {
			return out, nil
		}
		return nil, errors.New("no matching files found under stable/preview")
	}

	// Stability root case: contains version folders.
	urls, err := discoverLatestVersionFilesFromStabilityRoot(client, loc, includeGlobs, githubToken)
	if err != nil {
		return nil, err
	}
	return urls, nil
}

func orderedNonEmptyRoots(stableRoot, previewRoot string, opts deterministicDiscoveryOptions) []string {
	var roots []string
	// Only handle ordering here; any fallback behavior (e.g. trying preview when stable is missing)
	// is owned by the caller for clarity.
	roots = append(roots, stableRoot)
	if opts.IncludePreview {
		roots = append(roots, previewRoot)
	}
	var out []string
	seen := map[string]struct{}{}
	for _, r := range roots {
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

func inferSiblingStabilityRoots(dir string) (stableRoot, previewRoot string, ok bool) {
	clean := strings.TrimSuffix(dir, "/")
	parts := strings.Split(clean, "/")
	for i, p := range parts {
		if p != "stable" && p != "preview" {
			continue
		}
		serviceRoot := strings.Join(parts[:i], "/")
		if serviceRoot == "" {
			return "", "", false
		}
		return serviceRoot + "/stable", serviceRoot + "/preview", true
	}
	return "", "", false
}

func tryListMatchingFilesInDir(client *http.Client, loc githubLocation, includeGlobs []string, githubToken string) ([]string, bool) {
	items, err := getGitHubDirectoryContents(client, loc, githubToken)
	if err != nil {
		return nil, false
	}
	for _, g := range includeGlobs {
		urls := matchFiles(items, g)
		if len(urls) > 0 {
			sort.Strings(urls)
			return urls, true
		}
	}
	return nil, false
}

func matchFiles(items []githubContentsItem, includeGlob string) []string {
	var urls []string
	for _, item := range items {
		if item.Type != "file" || item.DownloadURL == "" {
			continue
		}
		matched, err := filepath.Match(includeGlob, item.Name)
		if err != nil {
			continue
		}
		if matched {
			urls = append(urls, item.DownloadURL)
		}
	}
	return urls
}

func findDirItem(items []githubContentsItem, name string) *githubContentsItem {
	for i := range items {
		if items[i].Type == "dir" && items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func discoverLatestVersionFilesFromStabilityRoot(client *http.Client, stabilityRoot githubLocation, includeGlobs []string, githubToken string) ([]string, error) {
	items, err := getGitHubDirectoryContents(client, stabilityRoot, githubToken)
	if err != nil {
		return nil, err
	}
	version := pickLatestVersionDirName(items)
	if version == "" {
		return nil, errors.New("no version directories found")
	}
	versionLoc := githubLocation{Owner: stabilityRoot.Owner, Repo: stabilityRoot.Repo, Ref: stabilityRoot.Ref, Dir: strings.TrimSuffix(stabilityRoot.Dir, "/") + "/" + version}
	versionItems, err := getGitHubDirectoryContents(client, versionLoc, githubToken)
	if err != nil {
		return nil, err
	}
	for _, g := range includeGlobs {
		urls := matchFiles(versionItems, g)
		if len(urls) == 0 {
			continue
		}
		sort.Strings(urls)
		return urls, nil
	}
	return nil, errors.New("no matching files found in latest version directory")
}

func pickLatestVersionDirName(items []githubContentsItem) string {
	bestName := ""
	var bestTime time.Time
	bestHasTime := false
	for _, item := range items {
		if item.Type != "dir" {
			continue
		}
		name := item.Name
		if t, ok := parseAPIVersionDatePrefix(name); ok {
			if !bestHasTime || t.After(bestTime) || (t.Equal(bestTime) && name > bestName) {
				bestName = name
				bestTime = t
				bestHasTime = true
			}
			continue
		}
		// Fallback when version isn't date-like: use lexical max for determinism.
		if !bestHasTime && name > bestName {
			bestName = name
		}
	}
	return bestName
}

func parseAPIVersionDatePrefix(versionName string) (time.Time, bool) {
	// Handles both stable (YYYY-MM-DD) and preview (YYYY-MM-DD-preview).
	if len(versionName) < 10 {
		return time.Time{}, false
	}
	prefix := versionName[:10]
	t, err := time.Parse("2006-01-02", prefix)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

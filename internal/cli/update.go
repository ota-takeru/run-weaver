package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var errUpdateRestarting = errors.New("run-weaver update is restarting")

var updateHTTPClient = &http.Client{Timeout: 30 * time.Second}
var checkReleaseUpdateFunc = checkReleaseUpdate
var installReleaseUpdateFunc = installReleaseUpdate

const updateRequestSchemaVersion = 1

type updateInfo struct {
	Available bool
	Version   string
	AssetName string
	AssetURL  string
}

type daemonUpdateRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	RequestedAt   string `json:"requestedAt"`
	TargetVersion string `json:"targetVersion,omitempty"`
	Source        string `json:"source"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func maybeAutoUpdateOnStartup(args []string, stderr io.Writer) (int, bool) {
	if !shouldAutoUpdate(args) {
		return exitOK, false
	}
	result, err := checkReleaseUpdateFunc(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "update warning: %v\n", err)
		return exitOK, false
	}
	if !result.Available {
		return exitOK, false
	}
	if err := installReleaseUpdateFunc(context.Background(), result, argsForRestart(args)); err != nil {
		if errors.Is(err, errUpdateRestarting) {
			fmt.Fprintf(stderr, "updated run-weaver to %s; restarting\n", result.Version)
			return exitOK, true
		}
		fmt.Fprintf(stderr, "update warning: %v\n", err)
		return exitOK, false
	}
	fmt.Fprintf(stderr, "updated run-weaver to %s; continuing with current process\n", result.Version)
	return exitOK, false
}

func shouldAutoUpdate(args []string) bool {
	if Version == "" || Version == "dev" {
		return false
	}
	if os.Getenv("RUN_WEAVER_NO_UPDATE") != "" {
		return false
	}
	if len(args) == 0 {
		return false
	}
	return args[0] == "daemon"
}

func writeDaemonUpdateRequests(targetVersion string) ([]string, error) {
	targets := defaultUpdateRequestTargets()
	written := make([]string, 0, len(targets))
	for _, target := range targets {
		path := updateRequestPath(target)
		if err := writeDaemonUpdateRequest(path, daemonUpdateRequest{
			SchemaVersion: updateRequestSchemaVersion,
			RequestedAt:   time.Now().UTC().Format(time.RFC3339),
			TargetVersion: targetVersion,
			Source:        "run-weaver update",
		}); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

func defaultUpdateRequestTargets() []string {
	if currentGOOS == "windows" {
		return []string{"windows"}
	}
	return []string{"wsl"}
}

func updateRequestPath(target string) string {
	return filepath.Join(defaultStateRoot(target), "update-request.json")
}

func writeDaemonUpdateRequest(path string, request daemonUpdateRequest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func daemonUpdateRequested(target string) bool {
	_, err := os.Stat(updateRequestPath(target))
	return err == nil
}

func clearDaemonUpdateRequest(target string) {
	_ = os.Remove(updateRequestPath(target))
}

func maybeApplyDaemonUpdateAtSafePoint(target string, restartArgs []string) (bool, error) {
	if !shouldApplyDaemonSafeUpdate(target) || !daemonUpdateRequested(target) {
		return false, nil
	}
	result, err := checkReleaseUpdateFunc(context.Background())
	if err != nil {
		return false, err
	}
	if !result.Available {
		clearDaemonUpdateRequest(target)
		return false, nil
	}
	if err := installReleaseUpdateFunc(context.Background(), result, argsForRestart(restartArgs)); err != nil {
		if errors.Is(err, errUpdateRestarting) {
			return true, nil
		}
		return false, err
	}
	clearDaemonUpdateRequest(target)
	return false, nil
}

func shouldApplyDaemonSafeUpdate(target string) bool {
	if target == "" || Version == "" || Version == "dev" {
		return false
	}
	return os.Getenv("RUN_WEAVER_NO_UPDATE") == ""
}

func checkReleaseUpdate(ctx context.Context) (updateInfo, error) {
	if Version == "" || Version == "dev" {
		return updateInfo{Available: false, Version: Version}, nil
	}
	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return updateInfo{}, err
	}
	if !isReleaseNewer(release.TagName, Version) {
		return updateInfo{Available: false, Version: release.TagName}, nil
	}
	assetName := releaseAssetName(currentGOOS, runtime.GOARCH)
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return updateInfo{
				Available: true,
				Version:   release.TagName,
				AssetName: asset.Name,
				AssetURL:  asset.BrowserDownloadURL,
			}, nil
		}
	}
	return updateInfo{}, fmt.Errorf("release %s has no asset %s", release.TagName, assetName)
}

func fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	url := "https://api.github.com/repos/" + releaseRepository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "run-weaver/"+Version)
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("GitHub Releases returned %s", resp.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	if release.TagName == "" {
		return githubRelease{}, errors.New("latest release has no tag")
	}
	return release, nil
}

func installReleaseUpdate(ctx context.Context, info updateInfo, restartArgs []string) error {
	if !info.Available {
		return nil
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "run-weaver-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, info.AssetName)
	if err := downloadFile(ctx, info.AssetURL, archivePath); err != nil {
		return err
	}
	extracted, err := extractReleaseBinary(archivePath, tempDir)
	if err != nil {
		return err
	}
	if err := os.Chmod(extracted, 0o755); err != nil {
		return err
	}
	nextPath := current + ".new"
	_ = os.Remove(nextPath)
	if err := copyFile(extracted, nextPath, 0o755); err != nil {
		return err
	}
	if currentGOOS == "windows" {
		return restartWithWindowsReplacement(current, nextPath, restartArgs)
	}
	if err := os.Rename(nextPath, current); err != nil {
		return err
	}
	return restartCurrentProcess(current, restartArgs)
}

func downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "run-weaver/"+Version)
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned %s", url, resp.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func extractReleaseBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipBinary(archivePath, destDir)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGzBinary(archivePath, destDir)
	}
	return "", fmt.Errorf("unsupported release asset format %s", filepath.Base(archivePath))
}

func extractZipBinary(archivePath, destDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !isReleaseBinaryName(filepath.Base(file.Name)) {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		return writeReleaseBinary(destDir, filepath.Base(file.Name), src)
	}
	return "", errors.New("release zip does not contain run-weaver binary")
}

func extractTarGzBinary(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg || !isReleaseBinaryName(filepath.Base(header.Name)) {
			continue
		}
		return writeReleaseBinary(destDir, filepath.Base(header.Name), tr)
	}
	return "", errors.New("release tarball does not contain run-weaver binary")
}

func writeReleaseBinary(destDir, name string, src io.Reader) (string, error) {
	out := filepath.Join(destDir, name)
	dst, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return out, nil
}

func isReleaseBinaryName(name string) bool {
	return name == "run-weaver" || name == "run-weaver.exe"
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func restartWithWindowsReplacement(binary, nextPath string, args []string) error {
	command := strings.Join([]string{
		"ping -n 2 127.0.0.1 > nul",
		"move /Y " + windowsCmdQuote(nextPath) + " " + windowsCmdQuote(binary) + " > nul",
		"start \"\" " + windowsCmdQuote(binary) + windowsCommandArgs(args),
	}, " & ")
	cmd := exec.Command("cmd", "/c", command)
	if err := cmd.Start(); err != nil {
		return err
	}
	return errUpdateRestarting
}

func windowsCommandArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, windowsCmdQuote(arg))
	}
	return " " + strings.Join(quoted, " ")
}

func argsForRestart(args []string) []string {
	if args == nil {
		if len(os.Args) <= 1 {
			return nil
		}
		return append([]string(nil), os.Args[1:]...)
	}
	return append([]string(nil), args...)
}

func releaseAssetName(goos, goarch string) string {
	if goos == "windows" {
		return "run-weaver_" + goos + "_" + goarch + ".zip"
	}
	return "run-weaver_" + goos + "_" + goarch + ".tar.gz"
}

func isReleaseNewer(latest, current string) bool {
	latestParts, latestOK := parseVersion(latest)
	currentParts, currentOK := parseVersion(current)
	if latestOK && currentOK {
		for i := range latestParts {
			if latestParts[i] != currentParts[i] {
				return latestParts[i] > currentParts[i]
			}
		}
		return false
	}
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v")
}

func parseVersion(value string) ([3]int, bool) {
	var out [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.Split(value, "-")[0]
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

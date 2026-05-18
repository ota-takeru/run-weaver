package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShouldAutoUpdateOnlyReleaseDaemon(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = "dev"
	if shouldAutoUpdate([]string{"daemon"}) {
		t.Fatal("dev build should not auto update")
	}

	Version = "v0.1.0"
	if !shouldAutoUpdate([]string{"daemon"}) {
		t.Fatal("daemon should auto update in release builds")
	}
	if shouldAutoUpdate([]string{"install"}) {
		t.Fatal("install should not auto update because install scripts already download latest")
	}
	if shouldAutoUpdate([]string{"status"}) {
		t.Fatal("status should not auto update")
	}
}

func TestCheckReleaseUpdateFindsMatchingAsset(t *testing.T) {
	originalVersion := Version
	originalClient := updateHTTPClient
	defer func() {
		Version = originalVersion
		updateHTTPClient = originalClient
	}()
	Version = "v0.1.0"
	updateHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v0.2.0","assets":[{"name":"` + releaseAssetName(currentGOOS, runtime.GOARCH) + `","browser_download_url":"https://example.test/asset"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	info, err := checkReleaseUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Version != "v0.2.0" || info.AssetURL != "https://example.test/asset" {
		t.Fatalf("info = %#v", info)
	}
}

func TestReleaseVersionComparison(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.2.0", "v0.1.9", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.1.9", "v0.2.0", false},
		{"v0.2.0", "0.1.0", true},
	}
	for _, tc := range cases {
		if got := isReleaseNewer(tc.latest, tc.current); got != tc.want {
			t.Fatalf("isReleaseNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestExtractReleaseBinaryFromTarGz(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "run-weaver_linux_amd64.tar.gz")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("binary")
	if err := tw.WriteHeader(&tar.Header{Name: "run-weaver", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := extractReleaseBinary(archivePath, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", got)
	}
}

func TestExtractReleaseBinaryFromZip(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "run-weaver_windows_amd64.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create("run-weaver.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	path, err := extractReleaseBinary(archivePath, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", got)
	}
}

func TestWriteDaemonUpdateRequestsUsesCurrentTarget(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))
	originalGOOS := currentGOOS
	currentGOOS = "linux"
	defer func() { currentGOOS = originalGOOS }()

	paths, err := writeDaemonUpdateRequests("v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != updateRequestPath("wsl") {
		t.Fatalf("paths = %#v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, `"targetVersion": "v0.2.0"`) || !strings.Contains(got, `"source": "run-weaver update"`) {
		t.Fatalf("request = %s", got)
	}
}

func TestRunUpdateRequestsDaemonRefresh(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))
	originalVersion := Version
	originalGOOS := currentGOOS
	originalCheck := checkReleaseUpdateFunc
	originalInstall := installReleaseUpdateFunc
	defer func() {
		Version = originalVersion
		currentGOOS = originalGOOS
		checkReleaseUpdateFunc = originalCheck
		installReleaseUpdateFunc = originalInstall
	}()
	Version = "v0.1.0"
	currentGOOS = "linux"
	checkReleaseUpdateFunc = func(context.Context) (updateInfo, error) {
		return updateInfo{Available: true, Version: "v0.2.0"}, nil
	}
	installReleaseUpdateFunc = func(context.Context, updateInfo, []string) error {
		return nil
	}

	var stdout, stderr bytes.Buffer
	if code := runUpdate(nil, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "daemon update requested") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !daemonUpdateRequested("wsl") {
		t.Fatal("daemon update request was not written")
	}
}

func TestDaemonSafeUpdateAppliesRequest(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))
	originalVersion := Version
	originalCheck := checkReleaseUpdateFunc
	originalInstall := installReleaseUpdateFunc
	defer func() {
		Version = originalVersion
		checkReleaseUpdateFunc = originalCheck
		installReleaseUpdateFunc = originalInstall
	}()
	Version = "v0.1.0"
	if err := writeDaemonUpdateRequest(updateRequestPath("wsl"), daemonUpdateRequest{SchemaVersion: updateRequestSchemaVersion, Source: "test"}); err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	checkReleaseUpdateFunc = func(context.Context) (updateInfo, error) {
		return updateInfo{Available: true, Version: "v0.2.0"}, nil
	}
	installReleaseUpdateFunc = func(_ context.Context, _ updateInfo, args []string) error {
		gotArgs = append([]string(nil), args...)
		return errUpdateRestarting
	}

	restarting, err := maybeApplyDaemonUpdateAtSafePoint("wsl", []string{"daemon", "--target", "wsl"})
	if err != nil {
		t.Fatal(err)
	}
	if !restarting {
		t.Fatal("daemon update should restart")
	}
	if strings.Join(gotArgs, " ") != "daemon --target wsl" {
		t.Fatalf("restart args = %#v", gotArgs)
	}
}

func TestDaemonSafeUpdateClearsStaleRequestWhenAlreadyCurrent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "state"))
	originalVersion := Version
	originalCheck := checkReleaseUpdateFunc
	defer func() {
		Version = originalVersion
		checkReleaseUpdateFunc = originalCheck
	}()
	Version = "v0.2.0"
	if err := writeDaemonUpdateRequest(updateRequestPath("wsl"), daemonUpdateRequest{SchemaVersion: updateRequestSchemaVersion, Source: "test"}); err != nil {
		t.Fatal(err)
	}
	checkReleaseUpdateFunc = func(context.Context) (updateInfo, error) {
		return updateInfo{Available: false, Version: "v0.2.0"}, nil
	}

	restarting, err := maybeApplyDaemonUpdateAtSafePoint("wsl", []string{"daemon"})
	if err != nil {
		t.Fatal(err)
	}
	if restarting {
		t.Fatal("already-current request should not restart")
	}
	if daemonUpdateRequested("wsl") {
		t.Fatal("stale update request was not cleared")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

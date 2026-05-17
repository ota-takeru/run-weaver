param(
    [ValidateSet("windows")]
    [string]$Target = "windows",
    [string]$RepoUrl,
    [string]$Repo,
    [string]$BinDir = "$env:LOCALAPPDATA\run-weaver\bin",
    [string]$PollInterval = "1m"
)

$ErrorActionPreference = "Stop"

$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
    "arm64"
} else {
    "amd64"
}

$repoName = "ota-takeru/run-weaver"
$asset = "run-weaver_windows_$arch.zip"
$url = "https://github.com/$repoName/releases/latest/download/$asset"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("run-weaver-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $zip = Join-Path $tmp $asset
    Invoke-WebRequest -Uri $url -OutFile $zip
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $binary = Join-Path $BinDir "run-weaver.exe"
    Copy-Item -Path (Join-Path $tmp "run-weaver.exe") -Destination $binary -Force

    $installArgs = @("install", "--target", $Target, "--poll-interval", $PollInterval)
    if ($RepoUrl) {
        $installArgs += @("--repo-url", $RepoUrl)
    }
    if ($Repo) {
        $installArgs += @("--repo", $Repo)
    }
    & $binary @installArgs
    Write-Host "run-weaver installed at $binary"
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

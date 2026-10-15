[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir,
  [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Root)) {
  $Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}
if ([string]::IsNullOrWhiteSpace($OutDir)) {
  $OutDir = Join-Path $Root 'dist'
}

foreach ($cacheDir in @(
    (Join-Path $Root '.gocache'),
    (Join-Path $Root '.gomodcache'),
    (Join-Path $Root '.gopath'),
    (Join-Path $Root '.npm-cache')
  )) {
  New-Item -ItemType Directory -Force -Path $cacheDir | Out-Null
}
if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
  $env:GOCACHE = Join-Path $Root '.gocache'
}
if ([string]::IsNullOrWhiteSpace($env:GOMODCACHE)) {
  $env:GOMODCACHE = Join-Path $Root '.gomodcache'
}
if ([string]::IsNullOrWhiteSpace($env:GOPATH)) {
  $env:GOPATH = Join-Path $Root '.gopath'
}
if ([string]::IsNullOrWhiteSpace($env:NPM_CONFIG_CACHE)) {
  $env:NPM_CONFIG_CACHE = Join-Path $Root '.npm-cache'
}

function Invoke-Checked {
  param(
    [Parameter(Mandatory = $true)][string]$Command,
    [Parameter(Mandatory = $true)][string[]]$Arguments
  )

  & $Command @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$Command failed with exit code $LASTEXITCODE"
  }
}

function Get-GitCommit {
  try {
    $commit = (& git -C $Root rev-parse HEAD).Trim()
    if ($commit) {
      return $commit
    }
  } catch {
    return 'unknown'
  }
  return 'unknown'
}

function Get-Sha256 {
  param([Parameter(Mandatory = $true)][string]$Path)
  return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Test-PythonBuildAvailable {
  try {
    & python -m build --version *> $null
    return $LASTEXITCODE -eq 0
  } catch {
    return $false
  }
}

function Add-Artifact {
  param(
    [Parameter(Mandatory = $true)][object]$List,
    [Parameter(Mandatory = $true)][string]$Path
  )

  $item = Get-Item -LiteralPath $Path
  $entry = [ordered]@{
      path     = $Path.Substring($Root.Length + 1).Replace('\', '/')
      size     = [int64]$item.Length
      sha256   = Get-Sha256 -Path $Path
      modified = $item.LastWriteTimeUtc.ToString('o')
    }
  if ($List -is [System.Collections.IList]) {
    $null = $List.Add($entry)
    return
  }
  throw 'artifact list is not writable'
}

& (Join-Path $PSScriptRoot 'sync-distribution-version.ps1') -Root $Root

$manifestPath = Join-Path $Root 'distribution\version.json'
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$version = [string]$manifest.version
$releaseTag = [string]$manifest.release_tag

Push-Location $Root
try {
  if (-not $SkipTests) {
    Invoke-Checked -Command go -Arguments @('test', './...')
  }

  $releaseRoot = Join-Path $OutDir ("release\" + $version)
  $goDir = Join-Path $releaseRoot 'go'
  $assetDir = Join-Path $releaseRoot 'assets'
  $npmDir = Join-Path $releaseRoot 'npm'
  $pythonDir = Join-Path $releaseRoot 'python'
  foreach ($dir in @($goDir, $assetDir, $npmDir, $pythonDir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }

  $commit = Get-GitCommit
  $buildDate = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  $ldflags = "-X main.version=$version -X main.buildCommit=$commit -X main.buildDate=$buildDate"
  $targets = @(
    @{ platform = 'linux'; goos = 'linux'; arch = 'amd64'; suffix = '' },
    @{ platform = 'linux'; goos = 'linux'; arch = 'arm64'; suffix = '' },
    @{ platform = 'darwin'; goos = 'darwin'; arch = 'amd64'; suffix = '' },
    @{ platform = 'darwin'; goos = 'darwin'; arch = 'arm64'; suffix = '' },
    @{ platform = 'windows'; goos = 'windows'; arch = 'amd64'; suffix = '.exe' },
    @{ platform = 'windows'; goos = 'windows'; arch = 'arm64'; suffix = '.exe' }
  )
  $previousGoos = $env:GOOS
  $previousGoarch = $env:GOARCH
  $previousCgo = $env:CGO_ENABLED
  try {
    $env:CGO_ENABLED = '0'
    foreach ($target in $targets) {
      $env:GOOS = $target.goos
      $env:GOARCH = $target.arch
      $name = "traceai-$($target.platform)-$($target.arch)$($target.suffix)"
      $path = Join-Path $assetDir $name
      Invoke-Checked -Command go -Arguments @('build', '-trimpath', "-ldflags=$ldflags", '-o', $path, './cmd/traceai')
    }
  } finally {
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    $env:CGO_ENABLED = $previousCgo
  }

  $checksumPath = Join-Path $releaseRoot 'SHA256SUMS'
  $checksumLines = Get-ChildItem -LiteralPath $assetDir -File | Sort-Object Name | ForEach-Object {
    "$(Get-Sha256 -Path $_.FullName)  $($_.Name)"
  }
  $checksumLines | Set-Content -LiteralPath $checksumPath -Encoding ascii

  $previousVersion = $env:TRACEAI_VERSION
  $previousBaseUrl = $env:TRACEAI_RELEASE_BASE_URL
  $previousNpmCache = $env:NPM_CONFIG_CACHE
  try {
    $env:TRACEAI_VERSION = $version
    $env:TRACEAI_RELEASE_BASE_URL = [string]$manifest.binary_base_url
    if ([string]::IsNullOrWhiteSpace($env:NPM_CONFIG_CACHE)) {
      $env:NPM_CONFIG_CACHE = Join-Path $Root '.npm-cache'
    }

    if (Get-Command npm -ErrorAction SilentlyContinue) {
      Push-Location (Join-Path $Root 'distribution\npm')
      try {
        Invoke-Checked -Command npm -Arguments @('pack', '--pack-destination', $npmDir)
      } finally {
        Pop-Location
      }
    } else {
      Write-Warning 'npm not found; skipped npm pack'
    }

    if ((Get-Command python -ErrorAction SilentlyContinue) -and (Test-PythonBuildAvailable)) {
      Push-Location (Join-Path $Root 'distribution\python')
      try {
        Invoke-Checked -Command python -Arguments @('-m', 'build', '--sdist', '--wheel', '--outdir', $pythonDir)
      } finally {
        Pop-Location
      }
    } else {
      Write-Warning 'python build tooling not found; skipped python build'
    }
  } finally {
    $env:TRACEAI_VERSION = $previousVersion
    $env:TRACEAI_RELEASE_BASE_URL = $previousBaseUrl
    $env:NPM_CONFIG_CACHE = $previousNpmCache
  }

  $artifacts = New-Object System.Collections.ArrayList
  Get-ChildItem -LiteralPath $assetDir -File | ForEach-Object {
    Add-Artifact -List $artifacts -Path $_.FullName
  }
  Add-Artifact -List $artifacts -Path $checksumPath
  Get-ChildItem -LiteralPath $releaseRoot -Recurse -File |
    ForEach-Object {
      if ($_.FullName -like '*.tgz' -or $_.FullName -like '*.whl' -or $_.FullName -like '*.tar.gz') {
        Add-Artifact -List $artifacts -Path $_.FullName
      }
    }

  $releaseManifest = [ordered]@{
    version      = $version
    release_tag  = $releaseTag
    commit       = $commit
    build_date   = $buildDate
    go_binary    = (Join-Path $assetDir 'traceai-linux-amd64').Substring($Root.Length + 1).Replace('\', '/')
    binaries     = @($targets | ForEach-Object { "assets/traceai-$($_.platform)-$($_.arch)$($_.suffix)" })
    artifacts    = $artifacts
    npm_version  = $version
    python_version = $version
  }
  $releaseManifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $releaseRoot 'manifest.json') -Encoding utf8

  Write-Host "release artifacts written to $releaseRoot"
} finally {
  Pop-Location
}

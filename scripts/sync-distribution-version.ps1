[CmdletBinding()]
param(
  [string]$Root
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Root)) {
  $Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}

function Update-Line {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Pattern,
    [Parameter(Mandatory = $true)][string]$Replacement
  )

  $lines = Get-Content -LiteralPath $Path
  $updated = New-Object System.Collections.Generic.List[string]
  $changed = $false
  foreach ($line in $lines) {
    if ($line -match $Pattern) {
      $updated.Add($Replacement) | Out-Null
      $changed = $true
    } else {
      $updated.Add($line) | Out-Null
    }
  }
  if (-not $changed) {
    throw "failed to update $Path"
  }
  [System.IO.File]::WriteAllText($Path, ($updated -join [Environment]::NewLine), (New-Object System.Text.UTF8Encoding $false))
}

$versionPath = Join-Path $Root 'distribution\version.json'
$manifest = Get-Content -Raw -LiteralPath $versionPath | ConvertFrom-Json
$version = [string]$manifest.version

$packagePath = Join-Path $Root 'distribution\npm\package.json'
$package = Get-Content -Raw -LiteralPath $packagePath | ConvertFrom-Json
$package.version = $version
[System.IO.File]::WriteAllText($packagePath, ($package | ConvertTo-Json -Depth 10), (New-Object System.Text.UTF8Encoding $false))

Update-Line -Path (Join-Path $Root 'distribution\python\pyproject.toml') -Pattern '^version\s*=\s*".*"$' -Replacement "version = `"$version`""
Update-Line -Path (Join-Path $Root 'distribution\python\src\traceai\__init__.py') -Pattern '^__version__\s*=\s*".*"$' -Replacement "__version__ = `"$version`""

Write-Host "synchronized distribution version to $version"

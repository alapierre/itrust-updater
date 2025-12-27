param(
  [switch]$Sign,

  # <<< USTAW TUTAJ THUMBPRINT CERTYFIKATU CODE SIGNING >>>
  [string]$CertThumbprint = "E29FEE5773FA7CFC1FEAF52FB1984F83D4AEDB2A",
# Nowy: 0bfc9bd8d46156e41ba0053db16bc877b1a5f9f1

  # Timestamp RFC3161 (nowsze podejście)
  [string]$TimestampUrl = "http://time.certum.pl/",

  # Nazwa wynikowego pliku
  [string]$AppName = "itrust-updater",

  # Katalog wyjściowy
  [string]$OutDir = "dist",

  # Zmienna wersji w Go
  [string]$VersionVar = "github.com/alapierre/itrust-updater/version.Version",

  # Ścieżka do pakietu main (jeśli main nie jest w katalogu roboczym)
  [string]$MainPackage = ".",

  [string]$FallbackVersion = "1.0.0"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-VersionFromGitTags {

  $tag = git tag -l "v*" --sort=version:refname | Select-Object -Last 1
  if (-not $tag) { return $FallbackVersion }
  return $tag -replace '^v',''
}

function Find-SignTool {
  $st = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($st) { return $st.Source }

  $vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
  if (-not (Test-Path $vswhere)) {
    throw "Nie znaleziono signtool.exe w PATH i brak vswhere.exe. Uruchom w 'Developer Command Prompt' albo zainstaluj VS/Build Tools + Windows SDK."
  }

  $vsPath = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
  if (-not $vsPath) {
    throw "vswhere nie znalazł instalacji VS z narzędziami C++."
  }

  $patterns = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe",
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x86\signtool.exe",
    (Join-Path $vsPath "SDK\Windows Kits\10\bin\*\x64\signtool.exe")
  )

  foreach ($pattern in $patterns) {
    $found = Get-ChildItem -Path $pattern -ErrorAction SilentlyContinue |
      Sort-Object FullName -Descending |
      Select-Object -First 1
    if ($found) { return $found.FullName }
  }

  throw "Nie udało się znaleźć signtool.exe. Doinstaluj Windows SDK lub uruchom w 'Developer Command Prompt'."
}

# --- BUILD ---
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$version = Get-VersionFromGitTags
$exePath = Join-Path $OutDir ("{0}.exe" -f $AppName)

$ldflags = "-w -s -X `"$VersionVar=$version`""

Write-Host "VERSION    : $version"
Write-Host "LDFLAGS    : $ldflags"
Write-Host "OUTPUT EXE : $exePath"

& go build -trimpath -ldflags $ldflags -o $exePath $MainPackage

Write-Host "Build OK."

# --- SIGN (optional) ---
if ($Sign) {
  if (-not $CertThumbprint -or $CertThumbprint.Trim().Length -eq 0) {
    throw "Brak -CertThumbprint. Ustaw thumbprint certyfikatu Code Signing."
  }

  $signtool = Find-SignTool
  Write-Host "SIGNTOOL   : $signtool"
  Write-Host "CERT SHA1  : $CertThumbprint"
  Write-Host "TIMESTAMP  : $TimestampUrl"

  # RFC3161 timestamp (nowe podejście)
  & $signtool sign `
    /sha1 $CertThumbprint `
    /fd sha256 `
    /tr $TimestampUrl `
    /td sha256 `
    /v `
    $exePath

  # weryfikacja (też minimalna, ale bardzo przydatna)
  & $signtool verify /pa /v $exePath

  Write-Host "Sign OK."
}

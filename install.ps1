# texforge installer (Windows / PowerShell).
#
# Fetches the tectonic engine, the texforge CLI, and the default font bundle,
# then registers the fonts so \setmainfont{Name} works. No system TeX needed.
#
# Usage:   powershell -ExecutionPolicy Bypass -File install.ps1
#   or:    irm https://raw.githubusercontent.com/m3di/texforge/main/install.ps1 | iex
[CmdletBinding()]
param(
  [string]$TectonicVersion = $(if ($env:TECTONIC_VERSION) { $env:TECTONIC_VERSION } else { "0.17.0" }),
  [string]$Ref = $(if ($env:TEXFORGE_REF) { $env:TEXFORGE_REF } else { "main" })
)
$ErrorActionPreference = "Stop"
$Repo = "m3di/texforge"
$Home_ = if ($env:TEXFORGE_HOME) { $env:TEXFORGE_HOME } else { Join-Path $env:USERPROFILE ".texforge" }
$Bin   = Join-Path $Home_ "bin"
$Fonts = Join-Path $Home_ "fonts"
$Dl    = Join-Path $Home_ "dl"

function Say($m)  { Write-Host "> $m" -ForegroundColor Cyan }
function Info($m) { Write-Host "  $m" }
function Die($m)  { Write-Host "error: $m" -ForegroundColor Red; exit 1 }

New-Item -ItemType Directory -Force -Path $Bin, $Fonts, $Dl | Out-Null

# --- 1. Detect arch ---------------------------------------------------------
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "x86" }
$tectTriple = "x86_64-pc-windows-msvc"   # tectonic ships x86_64 msvc for Windows

# --- 2. tectonic engine -----------------------------------------------------
$tectExe = Join-Path $Bin "tectonic.exe"
if ((Test-Path $tectExe) -and (& $tectExe --version 2>$null)) {
  Say "tectonic already present"
} else {
  Say "Installing tectonic $TectonicVersion ($tectTriple)"
  $zip = Join-Path $Dl "tectonic.zip"
  $url = "https://github.com/tectonic-typesetting/tectonic/releases/download/tectonic%40$TectonicVersion/tectonic-$TectonicVersion-$tectTriple.zip"
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  Expand-Archive -Path $zip -DestinationPath $Dl -Force
  Copy-Item (Join-Path $Dl "tectonic.exe") $tectExe -Force
  Info "tectonic -> $tectExe"
}

# --- 3. texforge CLI: release binary, else build with Go --------------------
$tfExe = Join-Path $Bin "texforge.exe"
$asset = "texforge_windows_$arch.exe"
$rel   = "https://github.com/$Repo/releases/latest/download/$asset"
if ((Test-Path $tfExe) -and (& $tfExe version 2>$null)) {
  Say "texforge already present"
} else {
  try {
    Invoke-WebRequest -Uri $rel -OutFile $tfExe -UseBasicParsing
    Say "Installed texforge CLI (release binary)"
  } catch {
    if (Get-Command go -ErrorAction SilentlyContinue) {
      Say "No release binary — building texforge from source with Go"
      $src = Join-Path $Dl "src"
      if (Test-Path (Join-Path $src ".git")) { git -C $src pull --quiet }
      else { git clone --depth 1 --branch $Ref "https://github.com/$Repo.git" $src }
      Push-Location $src
      go build -ldflags "-X main.Version=$Ref" -o $tfExe ./cmd/texforge
      Pop-Location
      Info "texforge -> $tfExe"
    } else {
      Die "no prebuilt binary for $asset and Go is not installed to build one"
    }
  }
}

# --- 4. Font bundle ---------------------------------------------------------
Say "Fetching font bundle"
$GF = "https://github.com/google/fonts/raw/main/ofl"
function Get-Font($base, $name) {
  $dest = Join-Path $Fonts $name
  if (-not (Test-Path $dest)) { Invoke-WebRequest -Uri "$base/$name" -OutFile $dest -UseBasicParsing; Info "font: $name" }
}
"FiraSans-Regular","FiraSans-Bold","FiraSans-Italic","FiraSans-BoldItalic" | % { Get-Font "$GF/firasans" "$_.ttf" }
"IBMPlexSerif-Regular","IBMPlexSerif-Bold","IBMPlexSerif-Italic" | % { Get-Font "$GF/ibmplexserif" "$_.ttf" }
"IBMPlexMono-Regular","IBMPlexMono-Bold" | % { Get-Font "$GF/ibmplexmono" "$_.ttf" }
$om = Join-Path $Fonts "OpenMoji-Black.ttf"
if (-not (Test-Path $om)) {
  Invoke-WebRequest -Uri "https://github.com/hfg-gmuend/openmoji/raw/master/font/OpenMoji-black-glyf/OpenMoji-black-glyf.ttf" -OutFile $om -UseBasicParsing
  Info "font: OpenMoji-Black.ttf"
}

# --- 5. Register fonts ------------------------------------------------------
Say "Installing fonts into your user font directory"
& $tfExe fonts --install

# --- Done -------------------------------------------------------------------
Say "Done."
if (-not ($env:Path -split ';' | Where-Object { $_ -eq $Bin })) {
  Info "Add texforge to PATH (current user):"
  Info "  [Environment]::SetEnvironmentVariable('Path', `"$Bin;`$env:Path`", 'User')"
}
Info "Try:  texforge doctor ; texforge render examples\report\main.tex"

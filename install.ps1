# Installs the Webnak CLI on Windows.
#   irm https://raw.githubusercontent.com/sathninduk/webnak-cli/main/install.ps1 | iex
# Set $env:WEBNAK_VERSION to pin a version.
$ErrorActionPreference = "Stop"
$repo = "sathninduk/webnak-cli"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
if ($env:WEBNAK_VERSION) { $tag = "v" + $env:WEBNAK_VERSION.TrimStart("v") }
else { $tag = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name }
$name = "webnak_windows_$arch.exe"
$base = "https://github.com/$repo/releases/download/$tag"
$dir = Join-Path $env:LOCALAPPDATA "Programs\webnak"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$tmp = Join-Path ([IO.Path]::GetTempPath()) $name
Write-Host "Downloading webnak $tag for windows/$arch..."
Invoke-WebRequest "$base/$name" -OutFile $tmp -UseBasicParsing
$sums = (Invoke-WebRequest "$base/checksums.txt" -UseBasicParsing).Content
$want = ($sums -split "`n" | Where-Object { $_ -match [regex]::Escape($name) + '$' } | ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
$got = (Get-FileHash $tmp -Algorithm SHA256).Hash.ToLower()
if (-not $want -or $want.ToLower() -ne $got) { Remove-Item $tmp; throw "webnak: checksum mismatch, not installing" }
Move-Item -Force $tmp (Join-Path $dir "webnak.exe")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ";" | Where-Object { $_ -eq $dir })) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  Write-Host "Added $dir to your PATH (open a new terminal)."
}
Write-Host "Installed webnak to $dir\webnak.exe. Next: webnak login"

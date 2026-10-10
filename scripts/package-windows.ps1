param([Parameter(Mandatory=$true)][string]$Version, [Parameter(Mandatory=$true)][string]$Output)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^[0-9A-Za-z.+-]+$') { throw 'Invalid version' }
if ((go env GOOS) -ne 'windows') { throw 'Use a native Windows build host' }
$arch = (go env GOARCH).Trim()
if ($arch -notin @('amd64','arm64')) { throw 'Unsupported architecture' }
$publicKey = $env:DING_UPDATE_PUBLIC_KEY
if ($publicKey -and $publicKey -notmatch '^[A-Za-z0-9+/=]+$') { throw 'Invalid public update key' }
$release = $env:DING_PACKAGE_RELEASE -eq '1'
if ($release -and (!$publicKey -or !$env:DING_SIGNING_THUMBPRINT)) { throw 'A release requires the update public key and a Windows signing identity' }
New-Item -ItemType Directory -Force $Output | Out-Null
$Output = (Resolve-Path $Output).Path
$archive = Join-Path $Output "ding_windows_$arch.tar.gz"
if (Test-Path $archive) { throw 'Refusing to overwrite an artifact' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ('ding-package-' + [guid]::NewGuid())
New-Item -ItemType Directory $temp | Out-Null
try {
  $env:CGO_ENABLED = '0'
  go build -trimpath -tags console,mcpui "-ldflags=-s -w -X main.version=$Version -X github.com/ding-labs/ding/internal/update.PublicKey=$publicKey" -o "$temp\ding.exe" ./cmd/ding
  if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
  Copy-Item LICENSE $temp
  [IO.File]::WriteAllText("$temp\installation-owner", "external`n", [Text.UTF8Encoding]::new($false))
  if ($release) {
    signtool sign /sha1 $env:DING_SIGNING_THUMBPRINT /fd SHA256 /tr 'https://timestamp.digicert.com' /td SHA256 "$temp\ding.exe"
    if ($LASTEXITCODE -ne 0) { throw 'Executable signing failed' }
    if ((Get-AuthenticodeSignature "$temp\ding.exe").Status -ne 'Valid') { throw 'Executable signature validation failed' }
  }
  & "$temp\ding.exe" version --json
  if ($LASTEXITCODE -ne 0) { throw 'Executable validation failed' }
  tar -C $temp -czf $archive ding.exe LICENSE
  if ($LASTEXITCODE -ne 0) { throw 'Archive creation failed' }
  $iscc = Get-Command ISCC.exe -ErrorAction SilentlyContinue
  if (!$iscc) { $iscc = Get-Item "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe" }
  $arguments = @("/DDingVersion=$Version", "/DDingArch=$arch", "/DPayload=$temp", "/O$Output")
  if ($release) {
    $sign = 'signtool sign /sha1 ' + $env:DING_SIGNING_THUMBPRINT + ' /fd SHA256 /tr https://timestamp.digicert.com /td SHA256 $f'
    $arguments += @('/DReleaseSigning=1', "/Sding=$sign")
  }
  & $iscc @arguments native/windows/ding.iss
  if ($LASTEXITCODE -ne 0) { throw 'Installer compilation failed' }
  if ($release -and (Get-AuthenticodeSignature "$Output\ding_windows_${arch}_setup.exe").Status -ne 'Valid') { throw 'Installer signature validation failed' }
} finally { Remove-Item -Recurse -Force $temp }

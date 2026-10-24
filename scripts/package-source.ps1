$ErrorActionPreference='Stop'
$root=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$release=Join-Path $root 'release\0.5.0'
New-Item -ItemType Directory -Path $release -Force | Out-Null
$archive=Join-Path $release 'Foldmark-source-0.5.0.zip'
if (-not $archive.StartsWith($release+[System.IO.Path]::DirectorySeparatorChar,[System.StringComparison]::OrdinalIgnoreCase)) { throw '源码包路径不在 release 内。' }
if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive }
& tar -a -cf $archive -C $root `
  --exclude=source/frontend/node_modules `
  --exclude=source/frontend/dist `
  --exclude=source/.mygo `
  --exclude='source/.mygo*.go' `
  --exclude='source/*.exe' `
  --exclude='*.log' `
  README.md .gitignore source scripts docs
if ($LASTEXITCODE -ne 0) { throw '源码包创建失败。' }
$paths=@(
  $archive,
  (Join-Path $release 'windows-amd64\Foldmark.exe'),
  (Join-Path $release 'windows-amd64\Foldmark Setup 0.5.0 amd64.exe'),
  (Join-Path $release 'windows-amd64\THIRD_PARTY_NOTICES.txt')
)
$checksums=foreach ($path in $paths) {
  if (-not (Test-Path -LiteralPath $path)) { throw "发布文件缺失：$path" }
  $relative=[System.IO.Path]::GetRelativePath($release,$path).Replace('\','/')
  '{0}  {1}' -f (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant(),$relative
}
[System.IO.File]::WriteAllLines((Join-Path $release 'SHA256SUMS.txt'),$checksums,[System.Text.UTF8Encoding]::new($false))
Write-Output "Source archive: $archive"

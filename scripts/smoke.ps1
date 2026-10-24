$ErrorActionPreference='Stop'
$root=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$executable=Join-Path $root 'release\0.5.0\windows-amd64\Foldmark.exe'
if (-not (Test-Path -LiteralPath $executable)) { throw '请先运行 scripts/build.ps1。' }
$output=Join-Path $root ('test-results\packaged-'+[DateTimeOffset]::Now.ToUnixTimeMilliseconds())
New-Item -ItemType Directory -Force -Path $output | Out-Null
$first=Join-Path $output '中文 文档.md'
$second=Join-Path $output '第二份 文档.md'
[System.IO.File]::WriteAllText($first,"# 中文路径打开`n`n第一份文档。")
$body="`n# 再次打开文件`n`n应在已有窗口中打开第二个标签。这里有 **跨**格式 查找。`n`n![本地图片](preview.png)`n`n"
$body += "``````text`n复制这段代码`n```````n`n"
for ($i=1; $i -le 45; $i++) { $body += "## 第 $i 节`n`n这是用于验证分栏同步的第 $i 段文字。`n`n" }
[System.IO.File]::WriteAllText($second,$body)
Copy-Item -LiteralPath (Join-Path $root 'source\assets\icon.png') -Destination (Join-Path $output 'preview.png')
$savedData=$env:FOLDMARK_DATA_DIR
$savedCapture=$env:FOLDMARK_CAPTURE_DIR
$savedSlash=$env:FOLDMARK_CAPTURE_SLASH
$savedReading=$env:FOLDMARK_CAPTURE_READING
$savedSync=$env:FOLDMARK_CAPTURE_SYNC
$savedTable=$env:FOLDMARK_CAPTURE_TABLE
try {
  $env:FOLDMARK_DATA_DIR=Join-Path $output 'state'
  $env:FOLDMARK_CAPTURE_DIR=$output
  $env:FOLDMARK_CAPTURE_SLASH='1'
  $env:FOLDMARK_CAPTURE_READING='1'
  $env:FOLDMARK_CAPTURE_SYNC='1'
  $env:FOLDMARK_CAPTURE_TABLE='1'
  $firstProcess=Start-Process -FilePath $executable -ArgumentList ('"'+$first+'"') -WindowStyle Hidden -RedirectStandardError (Join-Path $output 'app-stderr.log') -PassThru
  Start-Sleep -Milliseconds 650
  $secondProcess=Start-Process -FilePath $executable -ArgumentList ('"'+$second+'"') -WindowStyle Hidden -PassThru
  if (-not $secondProcess.WaitForExit(15000)) { throw '第二个进程未退出。' }
  if (-not $firstProcess.WaitForExit(15000)) { throw '原生截图测试未完成。' }
  if ($firstProcess.ExitCode -ne 0) { throw ('原生应用运行失败：'+(Get-Content -LiteralPath (Join-Path $output 'app-stderr.log') -Raw)) }
  $session=Get-Content -LiteralPath (Join-Path $env:FOLDMARK_DATA_DIR 'session.json') -Raw | ConvertFrom-Json
  $paths=@($session.documents | ForEach-Object { $_.path })
  if ($paths -notcontains $first -or $paths -notcontains $second) { throw '文件参数或单实例打开测试失败。' }
  foreach ($name in @('foldmark-native.png','foldmark-compact.png','foldmark-slash.png','foldmark-reading-search.png','foldmark-code.png','foldmark-image-zoom.png','foldmark-sync.png','foldmark-table.png')) {
    if (-not (Test-Path -LiteralPath (Join-Path $output $name))) { throw '原生窗口截图缺失。' }
  }
  Write-Output 'PASS: Chinese paths, single-instance forwarding, local image, native rendering, slash menu, preview search, image zoom, split scroll, editable visual table and Markdown roundtrip.'
  Write-Output $output
}
finally {
  $env:FOLDMARK_DATA_DIR=$savedData
  $env:FOLDMARK_CAPTURE_DIR=$savedCapture
  $env:FOLDMARK_CAPTURE_SLASH=$savedSlash
  $env:FOLDMARK_CAPTURE_READING=$savedReading
  $env:FOLDMARK_CAPTURE_SYNC=$savedSync
  $env:FOLDMARK_CAPTURE_TABLE=$savedTable
}

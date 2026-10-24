$ErrorActionPreference='Stop'
$projectRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$goCommand=Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $goExecutable=$goCommand.Source }
else { $goExecutable=Join-Path $projectRoot '.tools\go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExecutable)) { throw '请先安装 Go 1.27.1 或更新版本。' }
$env:PATH=(Split-Path -Parent $goExecutable)+';'+$env:PATH
$env:GOPATH=Join-Path $projectRoot '.cache\gopath'
$env:GOCACHE=Join-Path $projectRoot '.cache\go-build'
$env:CGO_ENABLED='0'
$env:GOWORK='off'
$nodeCommand=Get-Command node -ErrorAction SilentlyContinue
if (-not $nodeCommand) { throw '请先安装 Node.js 22.12 或更新版本。' }
$pnpmCommand=Get-Command pnpm -ErrorAction SilentlyContinue
if (-not $pnpmCommand) { throw '请先启用 pnpm（可运行 corepack enable）。' }
$pnpmExecutable=$pnpmCommand.Source

. (Join-Path $PSScriptRoot 'env.ps1')
Push-Location (Join-Path $projectRoot 'source\frontend')
try {
  & $pnpmExecutable install --frozen-lockfile
  if ($LASTEXITCODE -ne 0) { throw '前端依赖安装失败。' }
}
finally { Pop-Location }
Push-Location (Join-Path $projectRoot 'source')
try { & $goExecutable tool mygo dev; if ($LASTEXITCODE -ne 0) { throw '应用启动失败。' } }
finally { Pop-Location }

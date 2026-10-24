. (Join-Path $PSScriptRoot 'env.ps1')
Push-Location (Join-Path $projectRoot 'source\frontend')
try {
  & $pnpmExecutable install --frozen-lockfile
  if ($LASTEXITCODE -ne 0) { throw '前端依赖安装失败。' }
  & $pnpmExecutable run build
  if ($LASTEXITCODE -ne 0) { throw '前端构建失败。' }
  & ($nodeCommand.Source) (Join-Path $projectRoot 'scripts\generate-notices.mjs')
  if ($LASTEXITCODE -ne 0) { throw '第三方许可清单生成失败。' }
}
finally { Pop-Location }
Push-Location (Join-Path $projectRoot 'source')
try {
  & $goExecutable test ./...
  if ($LASTEXITCODE -ne 0) { throw '测试未通过。' }
  & $goExecutable vet ./...
  if ($LASTEXITCODE -ne 0) { throw '静态检查未通过。' }
  & $goExecutable tool mygo build -platform windows/amd64 -skip-build-command
  if ($LASTEXITCODE -ne 0) { throw '安装包构建失败。' }
}
finally { Pop-Location }

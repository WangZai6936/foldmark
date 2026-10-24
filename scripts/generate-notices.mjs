import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const packageStore = join(root, 'source', 'frontend', 'node_modules', '.pnpm')
const resources = join(root, 'source', 'resources')
if (!existsSync(packageStore)) throw new Error('Run pnpm install before generating notices')

const notices = [readFileSync(join(resources, 'GO_NOTICES.txt'), 'utf8').trimEnd(), '\n\nFrontend package notices (including build-time dependencies)\n']
const seen = new Set()
for (const entry of readdirSync(packageStore, { withFileTypes: true })) {
  if (!entry.isDirectory()) continue
  const modules = join(packageStore, entry.name, 'node_modules')
  if (!existsSync(modules)) continue
  for (const item of readdirSync(modules, { withFileTypes: true })) {
    const names = item.name.startsWith('@')
      ? readdirSync(join(modules, item.name)).map((name) => `${item.name}/${name}`)
      : [item.name]
    for (const name of names) {
      const directory = join(modules, name)
      const manifest = join(directory, 'package.json')
      if (!existsSync(manifest)) continue
      const meta = JSON.parse(readFileSync(manifest, 'utf8'))
      const id = `${meta.name}@${meta.version}`
      if (seen.has(id)) continue
      seen.add(id)
      const licenseFile = readdirSync(directory).find((file) => /^(LICENSE|LICENCE|COPYING)(\.[^.]+)?$/i.test(file))
      notices.push(`\n--- ${id} | ${typeof meta.license === 'string' ? meta.license : 'license in package'} ---\n`)
      notices.push(licenseFile ? readFileSync(join(directory, licenseFile), 'utf8').trimEnd() : 'See the package source for license details.')
      notices.push('\n')
    }
  }
}
writeFileSync(join(resources, 'THIRD_PARTY_NOTICES.txt'), notices.join('\n'), 'utf8')
console.log(`Generated notices for ${seen.size} frontend packages`)

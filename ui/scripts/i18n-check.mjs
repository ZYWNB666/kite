// i18n consistency check.
// 1. en.json and zh.json must expose exactly the same flattened key set.
// 2. Every literal `t('...')` / `i18n.t('...')` key in src must exist in both.
// Exits non-zero when drift is detected. Run via `pnpm run check:i18n`.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const localesDir = join(root, 'src', 'i18n', 'locales')

const en = JSON.parse(readFileSync(join(localesDir, 'en.json'), 'utf8'))
const zh = JSON.parse(readFileSync(join(localesDir, 'zh.json'), 'utf8'))

function flatten(obj, prefix = '', out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) flatten(v, key, out)
    else out[key] = v
  }
  return out
}

function fail(msg) {
  console.error(msg)
  process.exitCode = 1
}

// ── 1. parity between locales ────────────────────────────────────────────────
const enKeys = new Set(Object.keys(flatten(en)))
const zhKeys = new Set(Object.keys(flatten(zh)))
const onlyEn = [...enKeys].filter((k) => !zhKeys.has(k)).sort()
const onlyZh = [...zhKeys].filter((k) => !enKeys.has(k)).sort()

if (onlyEn.length) {
  fail(
    `i18n: ${onlyEn.length} key(s) only in en.json:\n  ${onlyEn.join('\n  ')}`
  )
}
if (onlyZh.length) {
  fail(
    `i18n: ${onlyZh.length} key(s) only in zh.json:\n  ${onlyZh.join('\n  ')}`
  )
}

// ── 2. literal t() keys must exist in both locales ──────────────────────────
const srcDir = join(root, 'src')
const files = []
function walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    const st = statSync(p)
    if (st.isDirectory()) {
      if (name === 'node_modules') continue
      walk(p)
    } else if (/\.(ts|tsx)$/.test(name)) {
      files.push(p)
    }
  }
}
walk(srcDir)

const used = new Map()
const re = /\b(?:t|i18n\.t)\(\s*['"]([^'"$]+)['"]/g
for (const file of files) {
  const text = readFileSync(file, 'utf8')
  let m
  while ((m = re.exec(text))) {
    if (!used.has(m[1])) used.set(m[1], [])
    used.get(m[1]).push(file)
  }
}

const missing = [...used.keys()]
  .filter((k) => !enKeys.has(k) || !zhKeys.has(k))
  .sort()
if (missing.length) {
  for (const k of missing) {
    fail(
      `i18n: key used in code but missing from a locale: "${k}" (${used.get(k)[0]})`
    )
  }
}

if (!process.exitCode) {
  console.log(
    `i18n ok: ${enKeys.size} keys, en/zh in sync, ${used.size} referenced keys resolved`
  )
}

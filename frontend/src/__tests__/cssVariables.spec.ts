// 自引用的自定义属性（--x: var(--x)）在 CSS 计算阶段即失效，属性变成
// guaranteed-invalid，用到它的背景会渲染成透明——页面上表现为卡片整块消失，
// 而构建、类型检查和 lint 全都不会报错。
// 这种写法只会在批量替换颜色值时误伤变量定义，所以固化成显式失败。
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')

function collectStyleSources(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    if (entry === '__tests__') return []
    const path = resolve(dir, entry)
    if (statSync(path).isDirectory()) return collectStyleSources(path)
    return /\.(vue|css)$/.test(entry) ? [path] : []
  })
}

// 注释里可能写着这个反例本身（用于警示），扫描前先剥掉，否则会误报。
// 用等长空白替换以保留行号。
function stripComments(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ' '))
}

describe('css custom properties', () => {
  it('never defines a custom property as a reference to itself', () => {
    const offenders: string[] = []

    for (const path of collectStyleSources(sourceRoot)) {
      const source = stripComments(readFileSync(path, 'utf8'))
      source.split('\n').forEach((line, index) => {
        const match = line.match(/(--[\w-]+)\s*:\s*var\(\s*(--[\w-]+)\s*[),]/)
        if (match && match[1] === match[2]) {
          offenders.push(`${path.replace(sourceRoot, 'src')}:${index + 1} ${match[1]}`)
        }
      })
    }

    expect(offenders).toEqual([])
  })
})

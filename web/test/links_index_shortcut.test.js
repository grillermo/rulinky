import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath, pathToFileURL } from 'node:url'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')

test('recognizes Cmd+Enter and Ctrl+Enter as create shortcuts', async () => {
  const { isCreateShortcut } = await import(pathToFileURL(resolve(webDir, 'src/pages/createShortcut.js')))
  const baseEvent = { key: 'Enter', metaKey: false, ctrlKey: false, shiftKey: false, altKey: false }

  assert.equal(isCreateShortcut({ ...baseEvent, metaKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, ctrlKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, shiftKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, altKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, key: 'a', metaKey: true }), false)
})

test('wires the shortcut handler and submit button hotkey hint', () => {
  const source = readFileSync(resolve(webDir, 'src/pages/LinksIndex.jsx'), 'utf8')

  assert.match(source, /onKeyDown=\{handleCreateShortcut\}/)
  assert.match(source, /Add link/)
  assert.match(source, /Cmd/)
  assert.match(source, /Enter/)
})

test('does not depend on Inertia', () => {
  for (const file of ['src/pages/LinksIndex.jsx', 'src/pages/AuthNew.jsx', 'src/pages/AuthVerify.jsx', 'src/main.jsx']) {
    const source = readFileSync(resolve(webDir, file), 'utf8')
    assert.doesNotMatch(source, /@inertiajs/, file)
  }
})

test('safeHref only allows http(s) URLs', async () => {
  const { safeHref } = await import(pathToFileURL(resolve(webDir, 'src/pages/safeHref.js')))
  assert.equal(safeHref('https://example.com/a?b=1'), 'https://example.com/a?b=1')
  assert.equal(safeHref('http://example.com'), 'http://example.com')
  for (const bad of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', ' javascript:alert(1)', 'data:text/html,<b>x</b>',
    'vbscript:x', 'file:///etc/passwd', '//evil.com', '/relative', 'not a url', '', null, undefined, 42]) {
    assert.equal(safeHref(bad), null, String(bad))
  }
})

test('LinksIndex renders hrefs only through safeHref', () => {
  const source = readFileSync(resolve(webDir, 'src/pages/LinksIndex.jsx'), 'utf8')
  assert.match(source, /href=\{safeHref\(link\.url\)\}/)
  assert.doesNotMatch(source, /href=\{link\.url\}/)
})

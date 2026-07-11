import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath, pathToFileURL } from 'node:url'

const testDir = dirname(fileURLToPath(import.meta.url))
const rootDir = resolve(testDir, '../..')

test('recognizes Cmd+Enter and Ctrl+Enter as create shortcuts', async () => {
  const shortcutModuleUrl = pathToFileURL(
    resolve(rootDir, 'app/javascript/Pages/Links/createShortcut.js')
  )
  const { isCreateShortcut } = await import(shortcutModuleUrl)
  const baseEvent = { key: 'Enter', metaKey: false, ctrlKey: false, shiftKey: false, altKey: false }

  assert.equal(isCreateShortcut({ ...baseEvent, metaKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, ctrlKey: true }), true)
  assert.equal(isCreateShortcut({ ...baseEvent, shiftKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, altKey: true }), false)
  assert.equal(isCreateShortcut({ ...baseEvent, key: 'a', metaKey: true }), false)
})

test('wires the shortcut handler and submit button hotkey hint', () => {
  const source = readFileSync(resolve(rootDir, 'app/javascript/Pages/Links/Index.jsx'), 'utf8')

  assert.match(source, /onKeyDown=\{handleCreateShortcut\}/)
  assert.match(source, /Add link/)
  assert.match(source, /Cmd/)
  assert.match(source, /Enter/)
})

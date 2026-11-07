import assert from 'node:assert/strict'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath, pathToFileURL } from 'node:url'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const { sendJSON } = await import(pathToFileURL(resolve(webDir, 'src/api.js')))

function stub(fetchImpl) {
  const assigned = []
  globalThis.fetch = fetchImpl
  globalThis.window = { location: { assign: url => assigned.push(url) } }
  return assigned
}

function reply(body, status = 200) {
  return async () => new Response(body, { status })
}

test('non-JSON 403 and 413 bodies do not throw', async () => {
  for (const status of [403, 413]) {
    stub(reply('Forbidden plain text', status))
    const result = await sendJSON('POST', '/links', { a: 1 })
    assert.equal(result.ok, false)
    assert.equal(result.status, status)
    assert.deepEqual(result.data, {})
  }
})

test('network failure resolves to ok:false, status 0', async () => {
  stub(async () => { throw new TypeError('failed to fetch') })
  const result = await sendJSON('GET', '/links')
  assert.equal(result.ok, false)
  assert.equal(result.status, 0)
})

test('follows a same-site redirect', async () => {
  const assigned = stub(reply(JSON.stringify({ redirect: '/x' })))
  const result = await sendJSON('POST', '/auth')
  assert.equal(result.ok, true)
  assert.deepEqual(assigned, ['/x'])
})

test('does not follow protocol-relative or absolute redirects', async () => {
  for (const redirect of ['//evil.com', 'https://evil.com']) {
    const assigned = stub(reply(JSON.stringify({ redirect })))
    await sendJSON('POST', '/auth')
    assert.deepEqual(assigned, [])
  }
})

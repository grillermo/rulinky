import { useState } from 'react'

const GENERIC_ERROR = 'Something went wrong. Please try again.'

// Only follow same-site paths from the server.
function isLocalPath(value) {
  return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//')
}

// JSON request to the Go server with the session cookie. Follows a
// { redirect } answer. Resolves to { ok, status, data }; never throws:
// non-JSON bodies (e.g. a plain-text 403 or 413) and network failures
// resolve to { ok: false, data: {} }.
export async function sendJSON(method, path, body) {
  let response
  try {
    response = await fetch(path, {
      method,
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
    })
  } catch {
    return { ok: false, status: 0, data: {} }
  }
  let data = {}
  try {
    const parsed = await response.json()
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) data = parsed
  } catch {
    data = {}
  }
  if (isLocalPath(data.redirect)) window.location.assign(data.redirect)
  return { ok: response.ok, status: response.status, data }
}

// Small replacement for Inertia's useForm: data, setData, errors,
// processing, reset, and submit(method, path) → sendJSON result.
// errors.base holds a generic message when the server gave no field errors.
export function useJSONForm(initial) {
  const [data, setDataState] = useState(initial)
  const [errors, setErrors] = useState({})
  const [processing, setProcessing] = useState(false)

  function setData(key, value) {
    setDataState(prev => ({ ...prev, [key]: value }))
  }

  function reset() {
    setDataState(initial)
  }

  async function submit(method, path) {
    setProcessing(true)
    try {
      const result = await sendJSON(method, path, data)
      if (result.ok) {
        setErrors({})
      } else {
        const fields = result.data.errors
        const hasFields = fields && typeof fields === 'object' && Object.keys(fields).length > 0
        setErrors(hasFields ? fields : { base: GENERIC_ERROR })
      }
      return result
    } finally {
      setProcessing(false)
    }
  }

  return { data, setData, errors, processing, reset, submit }
}

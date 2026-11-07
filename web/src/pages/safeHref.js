// Returns the URL when it is http(s), otherwise null, so user-saved links
// like javascript: or data: URLs are never rendered as a live href.
export function safeHref(value) {
  if (typeof value !== 'string') return null
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? value : null
  } catch {
    return null
  }
}

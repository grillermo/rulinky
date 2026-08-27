import { useState, useEffect } from 'react'
import { useForm, Link } from '@inertiajs/react'
import { isCreateShortcut } from './createShortcut'

const TIMELINE_BASE = [173, 27, 26] // rgba(173, 27, 26, 1)
const TIMELINE_END = [205, 196, 196] // muted, for the far end of the gradient
const TIMELINE_MAX_RANGE = 6 // buckets: 0 = this month ... 6 = "6+ months ago"

function monthsAgoIndex(ms) {
  if (!ms) return TIMELINE_MAX_RANGE
  const date = new Date(ms)
  const now = new Date()
  const diff = (now.getFullYear() - date.getFullYear()) * 12 + (now.getMonth() - date.getMonth())
  return Math.min(Math.max(diff, 0), TIMELINE_MAX_RANGE)
}

function rangeLabel(range) {
  if (range === 0) return 'This month'
  if (range === TIMELINE_MAX_RANGE) return `${TIMELINE_MAX_RANGE}+ months ago`
  return `${range} month${range > 1 ? 's' : ''} ago`
}

function rangeColor(range) {
  const t = range / TIMELINE_MAX_RANGE
  const [r, g, b] = TIMELINE_BASE.map((start, i) => Math.round(start + (TIMELINE_END[i] - start) * t))
  return `rgb(${r}, ${g}, ${b})`
}

function buildTimelineGroups(items) {
  const groups = []
  items.forEach(link => {
    const range = monthsAgoIndex(link.updatedAtMs)
    const last = groups[groups.length - 1]
    if (last && last.range === range) {
      last.items.push(link)
    } else {
      groups.push({ range, items: [link] })
    }
  })
  return groups
}

export default function LinksIndex({ links, readCount, unreadCount }) {
  const [filter, setFilter] = useState('unread')
  const [query, setQuery] = useState('')
  const [menuOpen, setMenuOpen] = useState(false)
  const [formOpen, setFormOpen] = useState(false)
  const [linksState, setLinksState] = useState(links)
  const [stickyReadIds, setStickyReadIds] = useState(() => new Set())
  const { data, setData, post, processing, errors, reset } = useForm({
    url: '',
    note: ''
  })

  useEffect(() => {
    setLinksState(links)
    setStickyReadIds(new Set())
  }, [links])

  const localReadCount = linksState.filter(l => l.read).length
  const localUnreadCount = linksState.length - localReadCount
  const displayedReadCount = linksState.length > 0 ? localReadCount : readCount
  const displayedUnreadCount = linksState.length > 0 ? localUnreadCount : unreadCount

  const queryTerms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)

  const filteredLinks = linksState.filter(link => {
    const matchesTab = filter === 'read' ? link.read : !link.read || stickyReadIds.has(link.id)
    if (!matchesTab) return false
    if (queryTerms.length === 0) return true
    const haystack = `${link.fullTitle || ''} ${link.note || ''} ${link.url || ''}`.toLowerCase()
    return queryTerms.every(term => haystack.includes(term))
  })

  function authToken() {
    const meta = document.querySelector('meta[name="rulinky-auth-token"]')
    return (meta && meta.getAttribute('content')) || ''
  }

  function setLinkReadState(id, nextRead, keepVisibleInUnread = false) {
    setLinksState(prev => prev.map(link => (link.id === id ? { ...link, read: nextRead } : link)))
    setStickyReadIds(prev => {
      const next = new Set(prev)
      if (!nextRead) {
        next.delete(id)
        return next
      }
      if (keepVisibleInUnread) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
  }

  function persistReadState(id, nextRead, keepalive = false) {
    return fetch('/api/links', {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        Authorization: authToken()
      },
      body: JSON.stringify({ id, read: nextRead }),
      keepalive
    }).then(response => {
      if (!response.ok) {
        throw new Error('Failed to update link')
      }
    })
  }

  function handleDelete(e, id) {
    e.preventDefault()
    e.stopPropagation()
    const snapshot = linksState
    const stickySnapshot = stickyReadIds
    setLinksState(prev => prev.filter(l => l.id !== id))
    setStickyReadIds(prev => { const next = new Set(prev); next.delete(id); return next })
    fetch('/api/links', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', Authorization: authToken() },
      body: JSON.stringify({ id })
    }).then(response => {
      if (!response.ok) throw new Error('Failed to delete link')
    }).catch(() => {
      setLinksState(snapshot)
      setStickyReadIds(stickySnapshot)
    })
  }

  function handleToggleRead(e, link) {
    e.preventDefault()
    e.stopPropagation()
    const nextRead = !link.read
    setLinkReadState(link.id, nextRead)
    persistReadState(link.id, nextRead).catch(() => {
      setLinkReadState(link.id, link.read, stickyReadIds.has(link.id))
    })
  }

  function handleLinkClick(link) {
    if (link.read) return
    setLinkReadState(link.id, true, true)
    persistReadState(link.id, true, true).catch(() => {
      setLinkReadState(link.id, false)
    })
  }

  function handleCreate(e) {
    e.preventDefault()
    post('/links', {
      preserveScroll: true,
      onSuccess: () => {
        reset()
        setFilter('unread')
        setFormOpen(false)
      }
    })
  }

  function handleCreateShortcut(e) {
    if (!isCreateShortcut(e)) return
    handleCreate(e)
  }

  return (
    <main className="min-h-screen bg-gray-100 text-gray-900 font-sans p-4 flex flex-col items-center">
      <div id="linksList" className="bg-white rounded-lg shadow-lg p-6 w-full max-w-md">
        <header className="relative text-center justify-center min-h-[70px] pb-6">
          <img
            src="https://guillermo-public.s3.us-east-1.amazonaws.com/file-to-s3-uploads/9253d692-d691-4b84-83e3-766c1b99f39b-Image.png?v=20260405"
            alt="Rulinky logo"
            className="h-auto w-auto inline-block max-w-[80px]"
          />
          <div className="absolute right-0 top-0">
            <button
              type="button"
              onClick={() => setMenuOpen(open => !open)}
              className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 hover:cursor-pointer"
              aria-label="Menu"
              aria-haspopup="true"
              aria-expanded={menuOpen}
            >
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                <line x1="3" y1="6" x2="17" y2="6" />
                <line x1="3" y1="10" x2="17" y2="10" />
                <line x1="3" y1="14" x2="17" y2="14" />
              </svg>
            </button>
            {menuOpen && (
              <div className="absolute right-0 mt-2 w-40 rounded-lg border border-gray-200 bg-white py-1 text-left shadow-lg z-20">
                <Link
                  href="/auth"
                  method="delete"
                  as="button"
                  className="block w-full px-4 py-2 text-left text-sm text-gray-700 transition-colors hover:bg-gray-100 hover:cursor-pointer"
                >
                  Sign out
                </Link>
              </div>
            )}
          </div>
        </header>

        <div>
          <div className="mb-6 rounded-lg border border-gray-200">
            <button
              type="button"
              onClick={() => setFormOpen(open => !open)}
              className="flex w-full items-center justify-between px-3 py-2 text-sm font-medium text-gray-700 hover:cursor-pointer hover:bg-gray-50"
              aria-expanded={formOpen}
              aria-controls="add-link-form"
            >
              <span>Add link</span>
              <svg
                width="14"
                height="14"
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className={`transition-transform ${formOpen ? 'rotate-180' : ''}`}
              >
                <polyline points="5 7.5 10 12.5 15 7.5" />
              </svg>
            </button>
            {formOpen && (
              <form
                id="add-link-form"
                onSubmit={handleCreate}
                onKeyDown={handleCreateShortcut}
                className="space-y-3 border-t border-gray-200 p-3"
              >
                <div>
                  <label htmlFor="new-link-url" className="sr-only">Link URL</label>
                  <input
                    id="new-link-url"
                    type="url"
                    value={data.url}
                    onChange={e => setData('url', e.target.value)}
                    placeholder="https://example.com/article"
                    autoFocus
                    className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100"
                  />
                  {errors.url && <p className="mt-1 text-sm text-red-600">{errors.url}</p>}
                </div>
                <div>
                  <label htmlFor="new-link-note" className="sr-only">Note</label>
                  <textarea
                    id="new-link-note"
                    value={data.note}
                    onChange={e => setData('note', e.target.value)}
                    placeholder="Optional note"
                    rows={2}
                    className="w-full resize-none rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100"
                  />
                  {errors.note && <p className="mt-1 text-sm text-red-600">{errors.note}</p>}
                </div>
                <button
                  type="submit"
                  disabled={processing}
                  className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-gray-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
                >
                  {processing ? 'Saving...' : (
                    <>
                      <span>Add link</span>
                      <span className="rounded bg-white/10 px-1.5 py-0.5 text-xs font-normal text-gray-300">Cmd+Enter</span>
                    </>
                  )}
                </button>
              </form>
            )}
          </div>

          <div className="relative mb-4">
            <label htmlFor="link-search" className="sr-only">Search links</label>
            <input
              id="link-search"
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              placeholder="Search links…"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 pr-9 text-sm shadow-sm focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery('')}
                className="absolute right-2 top-1/2 -translate-y-1/2 inline-flex h-6 w-6 items-center justify-center rounded-full text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 hover:cursor-pointer"
                aria-label="Clear search"
              >
                <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <line x1="5" y1="5" x2="15" y2="15" />
                  <line x1="15" y1="5" x2="5" y2="15" />
                </svg>
              </button>
            )}
          </div>

          <div className="flex p-1 bg-gray-100 rounded-lg mb-6 top-2 z-10 backdrop-blur-sm">
            <button
              onClick={() => setFilter('unread')}
              className={`pl-3 flex-1 py-2 text-sm font-medium rounded-md transition-all hover:cursor-pointer ${
                filter === 'unread' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Unread <span>{displayedUnreadCount}</span>
            </button>
            <button
              onClick={() => setFilter('read')}
              className={`pl-3 flex-1 py-2 text-sm font-medium rounded-md transition-all hover:cursor-pointer ${
                filter === 'read' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Read <span>{displayedReadCount}</span>
            </button>
          </div>

          <div className="min-h-[50vh] flex flex-col">
            {filteredLinks.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <p>No links found.</p>
              </div>
            ) : (
              buildTimelineGroups(filteredLinks).map(group => (
                <div key={`${group.range}-${group.items[0].id}`}>
                  <div className="sticky top-0 z-10 flex items-center gap-2 bg-white/95 pb-1.5 pt-3 backdrop-blur-sm">
                    <span
                      className="rounded-full px-2.5 py-0.5 text-[11px] font-semibold text-white"
                      style={{ background: rangeColor(group.range) }}
                    >
                      {rangeLabel(group.range)}
                    </span>
                    <span className="h-px flex-1 bg-gray-200" />
                  </div>
                  {group.items.map(link => (
                    <div key={link.id} className="relative flex gap-3">
                      <div className="relative flex w-5 shrink-0 justify-center">
                        <span className="absolute inset-y-0 left-1/2 w-0.5 -translate-x-1/2 bg-gray-200" />
                        <span
                          className="relative z-[1] mt-6 h-2.5 w-2.5 shrink-0 rounded-full border-2 border-white shadow-sm"
                          style={{ background: rangeColor(group.range) }}
                        />
                      </div>
                      <div className={`min-w-0 flex-1 py-2 rounded-xl border border-gray-200 transition-all${link.read ? ' bg-gray-100 opacity-60' : ''} link-wrapper`}>
                        <a
                          href={link.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          data-link-id={link.id}
                          data-link-read={link.read ? '1' : '0'}
                          className="block p-4 active:scale-[0.98] cursor-pointer text-inherit no-underline hover:cursor-pointer"
                          onClick={() => handleLinkClick(link)}
                        >
                          <div className="flex justify-between items-start gap-3">
                            <div className="flex-1 min-w-0">
                              <h3
                                className="text-lg font-medium text-gray-900 break-all"
                                title={link.fullTitle}
                              >
                                {link.title}
                              </h3>
                            </div>
                            <div className="flex shrink-0 flex-col items-center gap-2">
                              <button
                                type="button"
                                className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-red-200 bg-red-50 text-sm transition-colors hover:bg-red-100"
                                title="Delete link"
                                aria-label="Delete link"
                                onClick={e => handleDelete(e, link.id)}
                              >
                                🗑️
                              </button>
                              <button
                                type="button"
                                className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-blue-200 bg-blue-50 text-sm transition-colors hover:bg-blue-100"
                                data-role="toggle-read"
                                title={link.read ? 'Mark as unread' : 'Mark as read'}
                                aria-label={link.read ? 'Mark as unread' : 'Mark as read'}
                                onClick={e => handleToggleRead(e, link)}
                              >
                                {link.read ? '📩' : '✅'}
                              </button>
                            </div>
                          </div>
                        </a>
                        {link.note && (
                          <div className="mt-2 px-4 text-sm text-gray-600">
                            {link.note}
                          </div>
                        )}
                        <div className="mt-2 px-4 text-xs text-gray-400">
                          {link.updatedAt}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </main>
  )
}

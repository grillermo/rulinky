import { useState, useEffect } from 'react'
import { useForm, Link } from '@inertiajs/react'

export default function LinksIndex({ links, readCount, unreadCount }) {
  const [filter, setFilter] = useState('unread')
  const [menuOpen, setMenuOpen] = useState(false)
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

  const filteredLinks = linksState.filter(link => {
    if (filter === 'read') return link.read
    return !link.read || stickyReadIds.has(link.id)
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
      }
    })
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
          <form onSubmit={handleCreate} className="mb-6 space-y-3">
            <div>
              <label htmlFor="new-link-url" className="sr-only">Link URL</label>
              <input
                id="new-link-url"
                type="url"
                value={data.url}
                onChange={e => setData('url', e.target.value)}
                placeholder="https://example.com/article"
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
              className="w-full rounded-lg bg-gray-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              {processing ? 'Saving...' : 'Add link'}
            </button>
          </form>

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

          <div className="space-y-4 min-h-[50vh] flex flex-col">
            {filteredLinks.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <p>No links found.</p>
              </div>
            ) : (
              filteredLinks.map(link => (
                <div key={link.id} className={`rounded-xl border border-blue-200 transition-all${link.read ? ' bg-gray-100 opacity-60' : ''} link-wrapper`}>
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
              ))
            )}
          </div>
        </div>
      </div>
    </main>
  )
}

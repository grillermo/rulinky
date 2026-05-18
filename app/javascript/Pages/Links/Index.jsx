import { useState, useEffect } from 'react'
import { router } from '@inertiajs/react'

export default function LinksIndex({ links, readCount, unreadCount }) {
  const [filter, setFilter] = useState('unread')
  const [linkTitles, setLinkTitles] = useState({})

  useEffect(() => {
    const cleanups = []

    links.forEach(link => {
      if (!link.activeJobId) return

      let cancelled = false
      let timeoutId = null

      function poll() {
        if (cancelled) return

        fetch('/api/links/jobs/' + encodeURIComponent(link.activeJobId))
          .then(res => {
            if (!res.ok) throw new Error('failed')
            return res.json()
          })
          .then(payload => {
            if (cancelled) return
            if (!payload.finished) {
              timeoutId = setTimeout(poll, 2000)
              return
            }
            const raw = payload.link && payload.link.title ? payload.link.title : ''
            if (raw) {
              const title = raw.length > 30 ? raw.slice(0, 29) + '…' : raw
              setLinkTitles(prev => ({ ...prev, [link.id]: title }))
            }
          })
          .catch(() => {
            if (!cancelled) timeoutId = setTimeout(poll, 5000)
          })
      }

      poll()
      cleanups.push(() => { cancelled = true; clearTimeout(timeoutId) })
    })

    return () => cleanups.forEach(fn => fn())
  }, [links])

  const filteredLinks = links.filter(l => l.read === (filter === 'read'))

  function handleDelete(e, id) {
    e.preventDefault()
    e.stopPropagation()
    router.delete('/links/' + id, { preserveState: true })
  }

  function handleToggleRead(e, link) {
    e.preventDefault()
    e.stopPropagation()
    const path = link.read ? '/links/' + link.id + '/unread' : '/links/' + link.id + '/read'
    router.patch(path, {}, { preserveState: true })
  }

  return (
    <main className="min-h-screen bg-gray-100 text-gray-900 font-sans p-4 flex flex-col items-center">
      <div className="bg-white rounded-lg shadow-lg p-6 w-full max-w-md">
        <header className="text-center justify-center min-h-[70px] pb-6">
          <img
            src="https://guillermo-public.s3.us-east-1.amazonaws.com/file-to-s3-uploads/9253d692-d691-4b84-83e3-766c1b99f39b-Image.png?v=20260405"
            alt="Rulinky logo"
            className="h-auto w-auto inline-block max-w-[80px]"
          />
        </header>

        <div>
          <div className="flex p-1 bg-gray-100 rounded-lg mb-6 top-2 z-10 backdrop-blur-sm">
            <button
              onClick={() => setFilter('unread')}
              className={`pl-3 flex-1 py-2 text-sm font-medium rounded-md transition-all hover:cursor-pointer ${
                filter === 'unread' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Unread <span>{unreadCount}</span>
            </button>
            <button
              onClick={() => setFilter('read')}
              className={`pl-3 flex-1 py-2 text-sm font-medium rounded-md transition-all hover:cursor-pointer ${
                filter === 'read' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Read <span>{readCount}</span>
            </button>
          </div>

          <div className="space-y-1 min-h-[50vh]">
            {filteredLinks.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <p>No links found.</p>
              </div>
            ) : (
              filteredLinks.map(link => (
                <a
                  key={link.id}
                  href={link.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  data-link-id={link.id}
                  data-link-read={link.read ? '1' : '0'}
                  className={`block p-4 mb-3 rounded-xl border border-blue-200 transition-all active:scale-[0.98] cursor-pointer text-inherit no-underline hover:cursor-pointer${
                    link.read ? ' bg-gray-100 opacity-60' : ''
                  }`}
                >
                  <div className="flex justify-between items-start gap-3">
                    <div className="flex-1 min-w-0">
                      <h3
                        className="text-lg font-medium text-gray-900"
                        title={link.fullTitle}
                      >
                        {linkTitles[link.id] !== undefined ? linkTitles[link.id] : link.title}
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
                  <div className="mt-2 text-xs text-gray-400">
                    {link.updatedAt}
                  </div>
                </a>
              ))
            )}
          </div>
        </div>
      </div>
    </main>
  )
}

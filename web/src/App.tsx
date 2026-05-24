import { useEffect, useState } from 'react'
import DOMPurify from 'dompurify'

// Type definitions matching the Go model.Status
interface Tag {
  name: string
}

interface MediaAttachment {
  preview_url: string
  type: string
}

interface Account {
  acct: string
  display_name: string
  avatar: string
}

interface Status {
  id: string
  uri: string
  url: string
  created_at: string
  content: string
  language: string
  account: Account
  media_attachments: MediaAttachment[]
  tags: Tag[]
  reblog?: Status | null
}

function App() {
  const [statuses, setStatuses] = useState<Status[]>([])
  const [searchResults, setSearchResults] = useState<Status[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedStatus, setSelectedStatus] = useState<Status | null>(null)
  const [connectionStatus, setConnectionStatus] = useState<'connecting' | 'connected' | 'disconnected'>('disconnected')

  // Fetch initial timeline and setup SSE connection on mount
  useEffect(() => {
    fetchTimeline()
    const closeSSE = setupSSE()
    return () => {
      closeSSE()
    }
  }, [])

  const fetchTimeline = async () => {
    try {
      const resp = await fetch('/api/timeline?limit=50')
      if (resp.ok) {
        const data = await resp.json()
        setStatuses(data || [])
      }
    } catch (err) {
      console.error('Failed to fetch timeline:', err)
    }
  }

  const setupSSE = () => {
    setConnectionStatus('connecting')
    const eventSource = new EventSource('/api/stream')

    eventSource.onopen = () => {
      setConnectionStatus('connected')
    }

    eventSource.onerror = () => {
      setConnectionStatus('disconnected')
    }

    eventSource.addEventListener('status', (event: MessageEvent) => {
      try {
        const newStatus: Status = JSON.parse(event.data)
        setStatuses((prev) => {
          if (prev.some((s) => s.id === newStatus.id)) {
            return prev
          }
          const next = [newStatus, ...prev]
          if (next.length > 500) {
            return next.slice(0, 500)
          }
          return next
        })
      } catch (err) {
        console.error('Failed to parse SSE status:', err)
      }
    })

    return () => {
      eventSource.close()
    }
  }

  // Debounced search trigger (300ms delay)
  useEffect(() => {
    if (searchQuery.trim() === '') {
      setSearchResults([])
      return
    }

    const delayDebounceFn = setTimeout(() => {
      performSearch(searchQuery)
    }, 300)

    return () => clearTimeout(delayDebounceFn)
  }, [searchQuery])

  const performSearch = async (query: string) => {
    try {
      const resp = await fetch(`/api/search?q=${encodeURIComponent(query)}&limit=40`)
      if (resp.ok) {
        const data = await resp.json()
        setSearchResults(data.hits || [])
      }
    } catch (err) {
      console.error('Failed to perform search:', err)
    }
  }

  const formatTime = (timeStr: string) => {
    try {
      const date = new Date(timeStr)
      return date.toLocaleString()
    } catch {
      return timeStr
    }
  }

  const displayedStatuses = searchQuery.trim() !== '' ? searchResults : statuses

  return (
    <div className="app-container">
      <header className="app-header">
        <div className="brand">
          <span className="logo-icon">🪐</span>
          <h1>minidon</h1>
        </div>

        <div className="search-bar-container">
          <input
            id="search-input"
            type="text"
            placeholder="Search statuses (e.g. #golang, keywords)..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
          {searchQuery && (
            <button id="clear-search" className="clear-btn" onClick={() => setSearchQuery('')}>
              ✕
            </button>
          )}
        </div>

        <div className="connection-badge">
          <span className={`status-dot ${connectionStatus}`}></span>
          <span className="status-text">
            {connectionStatus === 'connected' && 'Live Connected'}
            {connectionStatus === 'connecting' && 'Connecting...'}
            {connectionStatus === 'disconnected' && 'Live Offline'}
          </span>
        </div>
      </header>

      <main className="main-content">
        {searchQuery.trim() !== '' && (
          <div className="search-meta">
            Showing results for <strong>"{searchQuery}"</strong> ({searchResults.length} found)
          </div>
        )}

        <div className="timeline-grid">
          {displayedStatuses.length === 0 ? (
            <div className="empty-state">
              <p>No statuses found. Wait for new stream updates or try another search.</p>
            </div>
          ) : (
            displayedStatuses.map((status) => {
              const actualStatus = status.reblog || status
              const isReblog = !!status.reblog
              const cleanHTML = DOMPurify.sanitize(actualStatus.content)

              return (
                <article
                  key={status.id}
                  id={`status-${status.id}`}
                  className="status-card"
                  onClick={() => setSelectedStatus(status)}
                >
                  {isReblog && (
                    <div className="reblog-header">
                      <span className="reblog-icon">🔁</span>
                      <span>{status.account.display_name || status.account.acct} boosted</span>
                    </div>
                  )}

                  <div className="card-header">
                    <img
                      src={actualStatus.account.avatar}
                      alt={actualStatus.account.display_name}
                      className="user-avatar"
                      onError={(e) => {
                        (e.target as HTMLImageElement).src = 'https://robohash.org/' + actualStatus.account.acct
                      }}
                    />
                    <div className="user-meta">
                      <span className="display-name">{actualStatus.account.display_name || actualStatus.account.acct}</span>
                      <span className="acct-name">@{actualStatus.account.acct}</span>
                    </div>
                    <time className="timestamp">{formatTime(actualStatus.created_at)}</time>
                  </div>

                  <div
                    className="card-content"
                    dangerouslySetInnerHTML={{ __html: cleanHTML }}
                  />

                  {actualStatus.media_attachments && actualStatus.media_attachments.length > 0 && (
                    <div className="media-preview-container">
                      {actualStatus.media_attachments.map((media, i) => (
                        <div key={i} className="media-preview-item">
                          {media.type === 'image' ? (
                            <img src={media.preview_url} alt="Attachment" className="media-preview-img" />
                          ) : (
                            <div className="media-other-type">🎥 {media.type}</div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}

                  {actualStatus.tags && actualStatus.tags.length > 0 && (
                    <div className="tags-container">
                      {actualStatus.tags.map((tag, i) => (
                        <span key={i} className="tag-pill">
                          #{tag.name}
                        </span>
                      ))}
                    </div>
                  )}
                </article>
              )
            })
          )}
        </div>
      </main>

      {/* Details Modal */}
      {selectedStatus && (
        <div className="modal-overlay" onClick={() => setSelectedStatus(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <header className="modal-header">
              <h2>Status Details</h2>
              <button id="close-modal" className="close-btn" onClick={() => setSelectedStatus(null)}>
                ✕
              </button>
            </header>
            <div className="modal-body">
              {(() => {
                const actual = selectedStatus.reblog || selectedStatus
                const cleanHTML = DOMPurify.sanitize(actual.content)
                return (
                  <>
                    <div className="modal-user-profile">
                      <img
                        src={actual.account.avatar}
                        alt={actual.account.display_name}
                        className="modal-avatar"
                        onError={(e) => {
                          (e.target as HTMLImageElement).src = 'https://robohash.org/' + actual.account.acct
                        }}
                      />
                      <div className="modal-user-meta">
                        <h3>{actual.account.display_name || actual.account.acct}</h3>
                        <p className="acct-name">@{actual.account.acct}</p>
                        <time className="modal-time">{formatTime(actual.created_at)}</time>
                      </div>
                    </div>

                    <div
                      className="modal-status-text"
                      dangerouslySetInnerHTML={{ __html: cleanHTML }}
                    />

                    {actual.media_attachments && actual.media_attachments.length > 0 && (
                      <div className="modal-media-gallery">
                        {actual.media_attachments.map((media, i) => (
                          <div key={i} className="modal-media-item">
                            {media.type === 'image' ? (
                              <img src={media.preview_url} alt="Attachment" className="modal-media-img" />
                            ) : (
                              <div className="modal-media-other">🎥 {media.type} preview available at <a href={actual.url} target="_blank" rel="noreferrer">Mastodon</a></div>
                            )}
                          </div>
                        ))}
                      </div>
                    )}

                    {actual.tags && actual.tags.length > 0 && (
                      <div className="modal-tags">
                        {actual.tags.map((tag, i) => (
                          <span key={i} className="modal-tag-pill">
                            #{tag.name}
                          </span>
                        ))}
                      </div>
                    )}

                    <div className="modal-footer-links">
                      <a href={actual.url} target="_blank" rel="noreferrer" className="btn-link">
                        View Original on Mastodon
                      </a>
                    </div>
                  </>
                )
              })()}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default App

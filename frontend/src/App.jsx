import { useState, useEffect, useCallback, useRef } from 'react'
import './index.css'

const API_BASE = ''
const EVENT_ID = 'modiji-meetup-2026'
const POLL_INTERVAL = 2000 // 2 seconds
const TTL_DURATION = 30 // seconds (should match backend hold TTL)

// Generate 100 default seats so the grid always renders even if backend is offline
const DEFAULT_SEATS = Array.from({ length: 100 }, (_, i) => ({
  id: String(i + 1),
  status: 'AVAILABLE',
}))

function App() {
  const [seats, setSeats] = useState(DEFAULT_SEATS)
  const [userId, setUserId] = useState('user-' + Math.random().toString(36).slice(2, 6))
  const [selectedSeat, setSelectedSeat] = useState(null)
  const [holdExpiry, setHoldExpiry] = useState(null)
  const [ttlRemaining, setTtlRemaining] = useState(0)
  const [backendStatus, setBackendStatus] = useState('checking')
  const [loading, setLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [toasts, setToasts] = useState([])
  const toastIdRef = useRef(0)

  // ── Toast helper ────────────────────────────────────
  const addToast = useCallback((message, type = 'info') => {
    const id = ++toastIdRef.current
    setToasts(prev => [...prev, { id, message, type }])
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id))
    }, 3000)
  }, [])

  // ── Fetch seats (polling) ───────────────────────────
  const fetchSeats = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/events/${EVENT_ID}/seats`)
      if (res.ok) {
        const data = await res.json()
        setSeats(data)
        setBackendStatus('online')

        // If the seat we're holding got released by TTL expiry on backend,
        // clear our local selection
        if (selectedSeat) {
          const ourSeat = data.find(s => s.id === selectedSeat)
          if (ourSeat && ourSeat.status === 'AVAILABLE') {
            // Our hold expired server-side
            setSelectedSeat(null)
            setHoldExpiry(null)
          } else if (ourSeat && ourSeat.status === 'BOOKED') {
            // Seat got booked (by us or someone else)
            setSelectedSeat(null)
            setHoldExpiry(null)
          }
        }
      } else {
        setBackendStatus('offline')
      }
    } catch {
      setBackendStatus('offline')
    } finally {
      setLoading(false)
    }
  }, [selectedSeat])

  useEffect(() => {
    fetchSeats()
    const interval = setInterval(fetchSeats, POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [fetchSeats])

  // ── TTL countdown timer ─────────────────────────────
  useEffect(() => {
    if (!holdExpiry) {
      setTtlRemaining(0)
      return
    }

    const tick = () => {
      const remaining = Math.max(0, (new Date(holdExpiry) - Date.now()) / 1000)
      setTtlRemaining(remaining)

      if (remaining <= 0) {
        // TTL expired
        setSelectedSeat(null)
        setHoldExpiry(null)
        addToast('Hold expired! Seat released back to the pool 💨', 'error')
      }
    }

    tick()
    const interval = setInterval(tick, 1000)
    return () => clearInterval(interval)
  }, [holdExpiry, addToast])

  // ── Hold a seat ─────────────────────────────────────
  const handleSeatClick = async (seat) => {
    if (seat.status !== 'AVAILABLE' || actionLoading) return
    if (selectedSeat) {
      addToast('Release current seat first!', 'info')
      return
    }

    setActionLoading(true)
    try {
      const res = await fetch(`${API_BASE}/events/${EVENT_ID}/hold`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seat_id: seat.id, user_id: userId }),
      })

      if (res.ok) {
        const data = await res.json()
        setSelectedSeat(seat.id)
        setHoldExpiry(data.expires_at)
        addToast(`Seat ${seat.id} held! Confirm before it expires 🕐`, 'success')
        fetchSeats()
      } else {
        const errText = await res.text()
        addToast(errText || 'Failed to hold seat', 'error')
      }
    } catch {
      addToast('Network error — could not hold seat', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  // ── Confirm booking ─────────────────────────────────
  const handleConfirm = async () => {
    if (!selectedSeat || actionLoading) return

    setActionLoading(true)
    try {
      const res = await fetch(`${API_BASE}/events/${EVENT_ID}/book`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seat_id: selectedSeat, user_id: userId }),
      })

      if (res.ok) {
        addToast(`Seat ${selectedSeat} booked! See you at the meetup 🎉`, 'success')
        setSelectedSeat(null)
        setHoldExpiry(null)
        fetchSeats()
      } else {
        const errText = await res.text()
        addToast(errText || 'Failed to book seat', 'error')
      }
    } catch {
      addToast('Network error — could not confirm booking', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  // ── Release hold ────────────────────────────────────
  const handleReject = async () => {
    if (!selectedSeat || actionLoading) return

    setActionLoading(true)
    try {
      const res = await fetch(`${API_BASE}/events/${EVENT_ID}/release`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seat_id: selectedSeat, user_id: userId }),
      })

      if (res.ok) {
        addToast(`Seat ${selectedSeat} released. Changed your mind? 🤔`, 'info')
        setSelectedSeat(null)
        setHoldExpiry(null)
        fetchSeats()
      } else {
        const errText = await res.text()
        addToast(errText || 'Failed to release seat', 'error')
      }
    } catch {
      addToast('Network error — could not release seat', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  // ── Seat class helper ───────────────────────────────
  const getSeatClass = (seat) => {
    if (seat.id === selectedSeat) return 'seat mine'
    switch (seat.status) {
      case 'HELD': return 'seat held'
      case 'BOOKED': return 'seat booked'
      default: return 'seat'
    }
  }

  // ── Counters ────────────────────────────────────────
  const availableCount = seats.filter(s => s.status === 'AVAILABLE').length
  const heldCount = seats.filter(s => s.status === 'HELD').length
  const bookedCount = seats.filter(s => s.status === 'BOOKED').length

  // ── TTL bar percentage ──────────────────────────────
  const ttlPercent = holdExpiry ? (ttlRemaining / TTL_DURATION) * 100 : 0

  // ── Row labels (A-J for 10 rows) ────────────────────
  const rowLabels = 'ABCDEFGHIJ'

  if (loading) {
    return (
      <div className="loading-container">
        <div className="spinner" />
        <p style={{ color: 'var(--text-secondary)' }}>
          Loading seats for the grand meetup...
        </p>
      </div>
    )
  }

  return (
    <>
      {/* ── Toasts ──────────────────────────────────── */}
      <div className="toast-container">
        {toasts.map(t => (
          <div key={t.id} className={`toast ${t.type}`}>
            {t.message}
          </div>
        ))}
      </div>

      {/* ── Hero ────────────────────────────────────── */}
      <section className="hero-section">
        <div className="event-tag">🪷 Once in a lifetime opportunity 🪷</div>
        <h1 className="hero-title">Modiji Meetup</h1>
        <p className="hero-subtitle">
          Secure your seat at the most <span className="highlight">exclusive</span> gathering of 2026.
          Double bookings? <span className="highlight">Not on our watch.</span>
        </p>
      </section>

      {/* ── Status Bar ──────────────────────────────── */}
      <div className="status-bar">
        <div className="status-indicator">
          <span className="status-dot available" />
          Available
        </div>
        <div className="status-indicator">
          <span className="status-dot held" />
          Held
        </div>
        <div className="status-indicator">
          <span className="status-dot booked" />
          Booked
        </div>
        <div className={`backend-pill ${backendStatus === 'online' ? 'online' : 'offline'}`}>
          ● API {backendStatus}
        </div>
      </div>

      {/* ── User ID ─────────────────────────────────── */}
      <div className="user-input-section">
        <label className="user-label" htmlFor="user-id-input">Your ID:</label>
        <input
          id="user-id-input"
          className="user-input"
          type="text"
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          placeholder="Enter your user ID"
          disabled={!!selectedSeat}
        />
      </div>

      {/* ── Counters ────────────────────────────────── */}
      <div className="seat-counters">
        <div className="counter-item">
          <div className="counter-value available-count">{availableCount}</div>
          <div className="counter-label">Available</div>
        </div>
        <div className="counter-item">
          <div className="counter-value held-count">{heldCount}</div>
          <div className="counter-label">Held</div>
        </div>
        <div className="counter-item">
          <div className="counter-value booked-count">{bookedCount}</div>
          <div className="counter-label">Booked</div>
        </div>
      </div>

      {/* ── Stage ───────────────────────────────────── */}
      <div className="stage-container">
        <div className="stage">🎤 Stage — Modiji Speaks Here 🎤</div>
      </div>

      {/* ── Seat Grid ───────────────────────────────── */}
      <div className="seat-grid-wrapper">
        <div className="seat-grid">
          {seats.map((seat, idx) => {
            const rowIdx = Math.floor(idx / 10)
            const colIdx = (idx % 10) + 1
            const label = `${rowLabels[rowIdx]}${colIdx}`

            return (
              <div
                key={seat.id}
                className={getSeatClass(seat)}
                onClick={() => handleSeatClick(seat)}
                title={`Seat ${label} — ${seat.id === selectedSeat ? 'YOUR PICK' : seat.status}`}
              >
                {label}
              </div>
            )
          })}
        </div>
      </div>

      {/* ── Selection Panel (slides up when seat held) */}
      <div className={`selection-panel ${selectedSeat ? 'visible' : ''}`}>
        <div className="selection-info">
          <div className="seat-label">
            🪑 Seat {selectedSeat
              ? `${rowLabels[Math.floor((parseInt(selectedSeat) - 1) / 10)]}${((parseInt(selectedSeat) - 1) % 10) + 1}`
              : '—'}
          </div>
          <div className="ttl-label">
            {ttlRemaining > 0
              ? `⏱ ${Math.ceil(ttlRemaining)}s remaining`
              : 'Expired'}
          </div>
          <div className="ttl-bar-track">
            <div
              className="ttl-bar-fill"
              style={{ width: `${ttlPercent}%` }}
            />
          </div>
        </div>
        <div className="selection-actions">
          <button
            className="btn btn-confirm"
            onClick={handleConfirm}
            disabled={actionLoading || ttlRemaining <= 0}
          >
            {actionLoading ? '...' : '✓ Confirm'}
          </button>
          <button
            className="btn btn-reject"
            onClick={handleReject}
            disabled={actionLoading}
          >
            ✗ Release
          </button>
        </div>
      </div>
    </>
  )
}

export default App

import { useState, useEffect } from 'react'

function App() {
  const [backendStatus, setBackendStatus] = useState('Checking...');

  useEffect(() => {
    // Check if the backend is reachable
    fetch('/healthz')
      .then(res => {
        if (res.ok) setBackendStatus('Online');
        else setBackendStatus('Error');
      })
      .catch(() => setBackendStatus('Offline'));
  }, []);

  return (
    <div className="dashboard-container">
      <header className="header">
        <h1>Seat Booking System</h1>
        <div className="status-badge" style={{ 
          color: backendStatus === 'Online' ? '#34d399' : '#f87171',
          background: backendStatus === 'Online' ? 'rgba(16, 185, 129, 0.1)' : 'rgba(248, 113, 113, 0.1)',
          borderColor: backendStatus === 'Online' ? 'rgba(52, 211, 153, 0.2)' : 'rgba(248, 113, 113, 0.2)'
        }}>
          Backend: {backendStatus}
        </div>
      </header>

      <main className="content-area">
        <div className="pulse-circle">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#3b82f6" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
          </svg>
        </div>
        <h2>Awaiting Backend Enhancements</h2>
        <p>
          The UI framework and design system are initialized. Once the backend supports listing users and events, the booking forms and dashboards will be integrated here.
        </p>
      </main>
    </div>
  )
}

export default App

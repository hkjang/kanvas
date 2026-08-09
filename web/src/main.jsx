import React, { useCallback, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { api, setCSRF } from './api'
import { Login } from './pages/Login'
import { Shell } from './components/Shell'
import { Home } from './pages/Home'
import { Space } from './pages/Space'
import { PageView } from './pages/PageView'
import { PageEdit } from './pages/PageEdit'
import { Personal } from './pages/Personal'
import { Admin } from './pages/Admin'
import { Logo } from './components/Logo'
import './styles.css'

function App() {
  const [account, setAccount] = useState(undefined)
  const [version, setVersion] = useState(null)

  const loadAccount = useCallback(async () => {
    try {
      const me = await api('/api/v1/me')
      setCSRF(me.csrfToken)
      setAccount(me)
    } catch (error) {
      if (error.status === 401) setAccount(null)
      else throw error
    }
  }, [])

  useEffect(() => {
    api('/api/v1/version').then(setVersion).catch(() => {})
    loadAccount().catch(() => setAccount(null))
  }, [loadAccount])

  if (account === undefined) return <div className="boot"><Logo /><span>Kanvas를 여는 중…</span></div>
  if (!account) return <Login version={version} onLogin={loadAccount} />

  return (
    <Routes>
      <Route element={<Shell account={account} version={version} onLogout={() => { setCSRF(''); setAccount(null) }} />}>
        <Route index element={<Home account={account} />} />
        <Route path="spaces/:spaceId" element={<Space />} />
        <Route path="pages/:pageId" element={<PageView />} />
        <Route path="pages/:pageId/edit" element={<PageEdit />} />
        <Route path="personal/*" element={<Personal account={account} version={version} />} />
        <Route path="admin/*" element={account.user.role === 'ADMIN' ? <Admin account={account} version={version} /> : <Navigate to="/" replace />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

createRoot(document.getElementById('root')).render(<BrowserRouter><App /></BrowserRouter>)

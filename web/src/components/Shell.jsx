import React, { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { Logo } from './Logo'
import { BookOpen, ChevronDown, Clock3, FilePlus2, Heart, LayoutGrid, Search, Settings, UserRound, X } from 'lucide-react'

export function Shell({ account, version, onLogout }) {
  const [spaces, setSpaces] = useState([])
  const [menu, setMenu] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const navigate = useNavigate()
  const searchTimer = useRef()
  useEffect(() => { api('/api/v1/spaces').then(v => setSpaces(v || [])).catch(() => {}) }, [])
  useEffect(() => {
    clearTimeout(searchTimer.current)
    if (!query.trim()) { setResults([]); return }
    searchTimer.current = setTimeout(() => api(`/api/v1/search?q=${encodeURIComponent(query)}`).then(v => setResults(v || [])).catch(() => {}), 220)
    return () => clearTimeout(searchTimer.current)
  }, [query])
  const logout = async () => { try { await api('/api/v1/auth/logout', { method: 'POST' }) } finally { onLogout(); navigate('/') } }
  return <div className="app-shell">
    <aside className="sidebar">
      <Link className="sidebar-brand" to="/"><Logo small /><strong>Kanvas</strong></Link>
      <nav className="main-nav">
        <NavLink to="/" end><LayoutGrid /> 홈</NavLink>
        <a href="#recent"><Clock3 /> 최근 문서</a>
        <a href="#favorites"><Heart /> 즐겨찾기</a>
      </nav>
      <div className="sidebar-section"><span>스페이스</span><button aria-label="스페이스 추가" onClick={() => navigate('/')}><FilePlus2 /></button></div>
      <nav className="space-nav">{spaces.map(space => <NavLink key={space.id} to={`/spaces/${space.id}`}><b>{space.key.slice(0, 2)}</b><span>{space.name}</span></NavLink>)}</nav>
      <div className="sidebar-foot"><BookOpen size={16} /><span>Confluence 호환 계층</span></div>
    </aside>
    <section className="workspace">
      <header className="topbar">
        <div className="global-search"><Search size={18} /><input value={query} onChange={e => setQuery(e.target.value)} placeholder="페이지, 스페이스 검색…" aria-label="글로벌 검색" />{query && <button onClick={() => setQuery('')}><X size={16} /></button>}
          {results.length > 0 && <div className="search-popover">{results.map(page => <button key={page.id} onClick={() => { navigate(`/pages/${page.id}`); setQuery(''); setResults([]) }}><strong>{page.title}</strong><small>{page.renderedText?.slice(0, 90)}</small></button>)}</div>}
        </div>
        <div className="profile-wrap">
          <button className="profile-button" onClick={() => setMenu(!menu)}><span>{initials(account.user.displayName)}</span><div><strong>{account.user.displayName}</strong><small>{account.user.role === 'ADMIN' ? '서비스 관리자' : '사용자'}</small></div><ChevronDown size={16} /></button>
          {menu && <div className="profile-menu">
            <div className="profile-summary"><span>{initials(account.user.displayName)}</span><div><strong>{account.user.displayName}</strong><small>{account.user.email || account.user.username}</small></div></div>
            <Link to="/personal" onClick={() => setMenu(false)}><UserRound /> 개인화 및 키 관리</Link>
            {account.user.role === 'ADMIN' && <Link to="/admin" onClick={() => setMenu(false)}><Settings /> Kanvas 서비스 관리</Link>}
            <div className="version-row"><span>Kanvas {version?.version}</span><small>{version?.commit?.slice(0, 8)}</small></div>
            <button className="logout" onClick={logout}>로그아웃</button>
          </div>}
        </div>
      </header>
      <div className="content"><Outlet context={{ account, spaces, setSpaces }} /></div>
    </section>
  </div>
}

function initials(name = '') { return name.split(/\s+/).map(v => v[0]).join('').slice(0, 2).toUpperCase() || 'K' }

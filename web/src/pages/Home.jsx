import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useOutletContext } from 'react-router-dom'
import { api, formatDate } from '../api'
import { ArrowRight, BookMarked, FilePlus2, FolderKanban, Gauge, MoveUpRight } from 'lucide-react'

export function Home({ account }) {
  const { spaces, setSpaces } = useOutletContext()
  const [recent, setRecent] = useState([])
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ key: '', name: '', description: '' })
  const navigate = useNavigate()
  useEffect(() => {
    if (!spaces[0]) { setRecent([]); return }
    api(`/api/v1/spaces/${spaces[0].id}/pages`).then(value => setRecent(value || [])).catch(() => setRecent([]))
  }, [spaces])
  const create = async e => { e.preventDefault(); const space = await api('/api/v1/spaces', { method: 'POST', body: JSON.stringify(form) }); setSpaces([...spaces, space]); setShowCreate(false); navigate(`/spaces/${space.id}`) }
  return <div className="page-frame home-page">
    <header className="welcome"><div><p className="eyebrow">KNOWLEDGE HOME</p><h1>안녕하세요, {account.user.displayName}님.</h1><p>팀의 지식을 찾고, 이어 쓰고, 안전하게 이전하세요.</p></div><button className="button primary" onClick={() => setShowCreate(true)}><FilePlus2 size={18} /> 새 스페이스</button></header>
    <section className="quick-grid">
      <article><span className="icon green"><FolderKanban /></span><div><small>활성 스페이스</small><strong>{spaces.length}</strong></div></article>
      <article><span className="icon amber"><BookMarked /></span><div><small>최근 문서</small><strong>{recent.length}</strong></div></article>
      <article><span className="icon blue"><Gauge /></span><div><small>데이터 모드</small><strong>Legacy-ready</strong></div></article>
    </section>
    <section className="section-card"><div className="section-heading"><div><h2>스페이스</h2><p>업무 영역별 지식 공간</p></div></div><div className="space-cards">{spaces.map(space => <Link key={space.id} to={`/spaces/${space.id}`}><span>{space.key.slice(0, 2)}</span><div><strong>{space.name}</strong><p>{space.description || '설명이 없습니다.'}</p><small>{formatDate(space.updatedAt)}</small></div><MoveUpRight size={18} /></Link>)}</div></section>
    <section className="section-card"><div className="section-heading"><div><h2>최근 문서</h2><p>다시 이어서 작업할 문서</p></div></div><div className="document-list">{recent.slice(0, 6).map(page => <Link key={page.id} to={`/pages/${page.id}`}><BookMarked /><div><strong>{page.title}</strong><small>{page.updatedBy} · {formatDate(page.updatedAt)}</small></div><ArrowRight /></Link>)}{recent.length === 0 && <div className="empty">아직 문서가 없습니다.</div>}</div></section>
    {showCreate && <div className="modal-backdrop"><form className="modal" onSubmit={create}><h2>새 스페이스</h2><p>팀이나 프로젝트를 위한 최상위 공간을 만듭니다.</p><label>키<input maxLength="16" placeholder="DEV" value={form.key} onChange={e => setForm({ ...form, key: e.target.value.toUpperCase() })} required /></label><label>이름<input placeholder="개발팀" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} required /></label><label>설명<textarea value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} /></label><div className="modal-actions"><button type="button" className="button secondary" onClick={() => setShowCreate(false)}>취소</button><button className="button primary">만들기</button></div></form></div>}
  </div>
}

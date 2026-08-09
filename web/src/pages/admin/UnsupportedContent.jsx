import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  CheckCircle2, ChevronLeft, ChevronRight, CircleAlert, FileSearch, RotateCcw,
  Search, ShieldCheck, Wrench, X,
} from 'lucide-react'
import { api, formatDate } from '../../api'

const pageSize = 100

export function UnsupportedContentCenter({ notify, confirm }) {
  const [result, setResult] = useState({ items: [], summary: { total: 0, open: 0, approved: 0, resolved: 0, byKind: {} }, filteredTotal: 0, limit: pageSize, offset: 0 })
  const [filters, setFilters] = useState({ q: '', status: '', kind: '' })
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [decision, setDecision] = useState(null)
  const [resolution, setResolution] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const query = new URLSearchParams({ limit: String(pageSize), offset: String(offset) })
      if (filters.q.trim()) query.set('q', filters.q.trim())
      if (filters.status) query.set('status', filters.status)
      if (filters.kind) query.set('kind', filters.kind)
      const value = await api(`/api/v1/admin/migration/unsupported?${query}`)
      setResult(value)
      setSelected([])
      setError('')
    } catch (loadError) {
      setError(loadError.message)
    } finally {
      setLoading(false)
    }
  }, [filters, offset])

  useEffect(() => {
    const timer = window.setTimeout(() => load(), 180)
    return () => window.clearTimeout(timer)
  }, [load])

  const items = result.items || []
  const summary = result.summary || { total: 0, open: 0, approved: 0, resolved: 0, byKind: {} }
  const filteredTotal = result.filteredTotal || 0
  const kinds = useMemo(() => Object.entries(summary.byKind || {}).sort(([left], [right]) => left.localeCompare(right)), [summary.byKind])
  const allSelected = items.length > 0 && items.every(item => selected.includes(item.id))

  const toggleAll = () => setSelected(allSelected ? [] : items.map(item => item.id))
  const toggle = id => setSelected(current => current.includes(id) ? current.filter(value => value !== id) : [...current, id])

  const applyDecision = async (ids, status, rationale) => {
    setBusy(true)
    try {
      await api('/api/v1/admin/migration/unsupported/bulk', {
        method: 'POST',
        body: JSON.stringify({ ids, status, resolution: rationale }),
      })
      setDecision(null)
      setResolution('')
      notify(`${ids.length}개 예외를 ${decisionLabel(status)} 상태로 변경했습니다.`)
      await load()
    } catch (decisionError) {
      notify(decisionError.message, 'error')
    } finally {
      setBusy(false)
    }
  }

  const requestDecision = async (ids, status) => {
    if (!ids.length) return
    if (status === 'OPEN') {
      const approved = await confirm({ title: '예외를 다시 열까요?', message: '현재 Cutover 근거에서 승인 또는 해결로 인정된 항목이 다시 미해결 상태가 됩니다.', danger: true, confirmLabel: '다시 열기' })
      if (approved) await applyDecision(ids, status, '')
      return
    }
    setResolution('')
    setDecision({ ids, status })
  }

  const submitDecision = async event => {
    event.preventDefault()
    await applyDecision(decision.ids, decision.status, resolution)
  }

  const updateFilters = next => {
    setOffset(0)
    setFilters(current => ({ ...current, ...next }))
  }

  return <>
    <header className="admin-header">
      <div><p className="eyebrow">UNSUPPORTED CONTENT CENTER</p><h1>예외 콘텐츠</h1><p>변환할 수 없거나 관계가 불완전한 콘텐츠를 검토하고 Cutover 위험 수용 근거를 남깁니다.</p></div>
      <div className="admin-header-actions"><button className="button secondary" onClick={load} disabled={loading}><RotateCcw /> 새로고침</button></div>
    </header>

    <section className="admin-metrics exception-metrics">
      <ExceptionMetric label="전체 예외" value={summary.total} note="최신 Snapshot" icon={FileSearch} />
      <ExceptionMetric label="미해결" value={summary.open} note="Cutover 차단" tone="danger" icon={CircleAlert} />
      <ExceptionMetric label="위험 승인" value={summary.approved} note="관리자 근거 보존" tone="amber" icon={ShieldCheck} />
      <ExceptionMetric label="해결 완료" value={summary.resolved} note="수동·변환 조치 완료" tone="green" icon={CheckCircle2} />
    </section>

    <section className="admin-card exception-policy">
      <ShieldCheck />
      <div><strong>Fail-closed 예외 정책</strong><p>미해결 항목은 검증을 통과할 수 없습니다. 위험 승인은 실제 변환을 의미하지 않으며, 입력한 사유와 관리자가 감사 로그에 기록됩니다.</p></div>
      {result.snapshotJobId && <code>{result.snapshotJobId.slice(0, 12)}</code>}
    </section>

    <section className="admin-card admin-toolbar exception-toolbar">
      <div className="admin-search"><Search /><input value={filters.q} onChange={event => updateFilters({ q: event.target.value })} placeholder="Macro, 유형, Legacy ID, 오류 내용 검색" aria-label="예외 콘텐츠 검색" />{filters.q && <button onClick={() => updateFilters({ q: '' })} aria-label="검색어 지우기"><X /></button>}</div>
      <select value={filters.status} onChange={event => updateFilters({ status: event.target.value })} aria-label="예외 상태 필터"><option value="">모든 상태</option><option value="OPEN">미해결</option><option value="APPROVED">위험 승인</option><option value="RESOLVED">해결 완료</option></select>
      <select value={filters.kind} onChange={event => updateFilters({ kind: event.target.value })} aria-label="예외 유형 필터"><option value="">모든 유형</option>{kinds.map(([kind, count]) => <option value={kind} key={kind}>{kind} ({count})</option>)}</select>
    </section>

    {selected.length > 0 && <section className="bulk-decision-bar" role="region" aria-label="선택 항목 일괄 조치"><strong>{selected.length}개 선택</strong><span>선택한 예외에 동일한 감사 근거를 적용합니다.</span><button className="button secondary" onClick={() => requestDecision(selected, 'APPROVED')} disabled={busy}><ShieldCheck /> 위험 승인</button><button className="button primary" onClick={() => requestDecision(selected, 'RESOLVED')} disabled={busy}><Wrench /> 해결 완료</button><button className="button ghost" onClick={() => requestDecision(selected, 'OPEN')} disabled={busy}><RotateCcw /> 다시 열기</button></section>}

    <section className="admin-card exception-table">
      <div className="exception-head"><label><input type="checkbox" checked={allSelected} onChange={toggleAll} aria-label="현재 페이지 전체 선택" /></label><span>유형 및 콘텐츠</span><span>발생</span><span>상태</span><span>조치</span></div>
      {loading ? <ExceptionLoading /> : error ? <div className="admin-error-state compact"><CircleAlert /><strong>예외 목록을 불러오지 못했습니다.</strong><p>{error}</p><button className="button secondary" onClick={load}>다시 시도</button></div> : items.map(item => <div className={`exception-row ${item.status.toLowerCase()}`} key={item.id}>
        <label><input type="checkbox" checked={selected.includes(item.id)} onChange={() => toggle(item.id)} aria-label={`${item.name} 선택`} /></label>
        <div className="exception-content"><span><b>{item.kind}</b><code>{item.legacyId || 'no legacy id'}</code></span><strong>{item.name}</strong><p>{item.sample || '상세 표본이 없습니다.'}</p>{item.resolution && <small><ShieldCheck /> {item.resolution} · {formatDate(item.resolvedAt)}</small>}{item.pageId && <Link to={`/pages/${item.pageId}`}>대상 페이지 열기 <ChevronRight /></Link>}</div>
        <strong className="exception-count">{Number(item.occurrenceCount || 0).toLocaleString('ko-KR')}</strong>
        <ExceptionState status={item.status} />
        <div className="exception-actions">{item.status !== 'APPROVED' && <button className="icon-action" onClick={() => requestDecision([item.id], 'APPROVED')} title="위험 승인"><ShieldCheck /></button>}{item.status !== 'RESOLVED' && <button className="icon-action" onClick={() => requestDecision([item.id], 'RESOLVED')} title="해결 완료"><Wrench /></button>}{item.status !== 'OPEN' && <button className="icon-action" onClick={() => requestDecision([item.id], 'OPEN')} title="다시 열기"><RotateCcw /></button>}</div>
      </div>)}
      {!loading && !error && items.length === 0 && <div className="admin-empty"><CheckCircle2 /><strong>조건에 맞는 예외가 없습니다.</strong><p>필터를 변경하거나 다음 Snapshot 결과를 기다리세요.</p></div>}
      <footer className="exception-pagination"><span>{filteredTotal ? `${offset + 1}–${Math.min(offset + items.length, filteredTotal)}` : '0'} · 검색 {filteredTotal} / 전체 {summary.total}</span><button className="button ghost" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - pageSize))}><ChevronLeft /> 이전</button><button className="button ghost" disabled={offset + items.length >= filteredTotal || loading} onClick={() => setOffset(offset + pageSize)}>다음 <ChevronRight /></button></footer>
    </section>

    {decision && <div className="modal-backdrop" onMouseDown={() => !busy && setDecision(null)}><form className="modal exception-decision" onSubmit={submitDecision} onMouseDown={event => event.stopPropagation()}><div className="modal-heading"><span className={decision.status === 'APPROVED' ? 'warning' : ''}>{decision.status === 'APPROVED' ? <ShieldCheck /> : <Wrench />}</span><div><h2>{decision.status === 'APPROVED' ? '위험을 승인할까요?' : '해결 완료로 표시할까요?'}</h2><p>{decision.ids.length}개 예외에 적용되며 감사 로그에 영구 기록됩니다.</p></div></div><label>조치 근거<textarea value={resolution} onChange={event => setResolution(event.target.value)} maxLength={2000} rows={5} required autoFocus placeholder={decision.status === 'APPROVED' ? '잔여 위험, 영향 범위와 승인 근거를 입력하세요.' : '수동 수정 또는 변환 조치 결과를 입력하세요.'} /></label><small>{resolution.length} / 2000</small><div className="modal-actions"><button type="button" className="button secondary" onClick={() => setDecision(null)} disabled={busy}>취소</button><button className={`button ${decision.status === 'APPROVED' ? 'warning' : 'primary'}`} disabled={busy || !resolution.trim()}>{busy ? '적용 중…' : decisionLabel(decision.status)}</button></div></form></div>}
  </>
}

function ExceptionMetric({ label, value, note, tone = '', icon: Icon }) {
  return <article className={tone}><Icon /><small>{label}</small><strong>{Number(value || 0).toLocaleString('ko-KR')}</strong><span>{note}</span></article>
}

function ExceptionState({ status }) {
  return <span className={`exception-state ${status.toLowerCase()}`}>{status === 'OPEN' ? '미해결' : status === 'APPROVED' ? '위험 승인' : '해결 완료'}</span>
}

function ExceptionLoading() {
  return <div className="exception-loading">{[0, 1, 2, 3].map(value => <div key={value}><i /><span><i /><i /></span><i /><i /></div>)}</div>
}

function decisionLabel(status) {
  return status === 'APPROVED' ? '위험 승인' : status === 'RESOLVED' ? '해결 완료' : '미해결'
}

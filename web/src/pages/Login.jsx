import React, { useEffect, useState } from 'react'
import { api } from '../api'
import { Logo } from '../components/Logo'
import { ArrowRight, KeyRound, ShieldCheck } from 'lucide-react'

export function Login({ version, onLogin }) {
  const [config, setConfig] = useState({ oidcEnabled: false, localLoginEnabled: true })
  const [form, setForm] = useState({ username: '', password: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api('/api/v1/auth/config').then(setConfig).catch(() => {}) }, [])

  const submit = async (event) => {
    event.preventDefault(); setError(''); setBusy(true)
    try { await api('/api/v1/auth/login', { method: 'POST', body: JSON.stringify(form) }); await onLogin() }
    catch (e) { setError(e.message) }
    finally { setBusy(false) }
  }

  return <main className="login-page">
    <section className="login-story">
      <div className="brand-lockup"><Logo /><strong>{config.product || 'Kanvas'}</strong></div>
      <div className="story-copy">
        <p className="eyebrow">KNOWLEDGE, CONTINUED</p>
        <h1>지식을 옮기고,<br />더 나은 문서로 이어가세요.</h1>
        <p>Confluence 호환 계층과 안전한 온라인 마이그레이션을 갖춘 사내 Wiki 플랫폼입니다.</p>
      </div>
      <div className="trust-row"><span><ShieldCheck size={18} /> ACL 일관성</span><span><KeyRound size={18} /> 개인 키 회전</span></div>
    </section>
    <section className="login-panel">
      <div className="login-card">
        <div><p className="eyebrow">WELCOME BACK</p><h2>{config.product || 'Kanvas'} 로그인</h2><p className="muted">계속하려면 사내 계정으로 인증하세요.</p></div>
        {config.oidcEnabled && <a className="button oidc-button" href="/api/v1/auth/oidc/login"><ShieldCheck size={18} /> Keycloak SSO로 계속 <ArrowRight size={17} /></a>}
        {config.oidcEnabled && <div className="divider"><span>또는 비상 관리자 계정</span></div>}
        <form onSubmit={submit}>
          <label>사용자 이름<input autoComplete="username" value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} required autoFocus /></label>
          <label>비밀번호<input type="password" autoComplete="current-password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required /></label>
          {error && <div className="error-box" role="alert">{error}</div>}
          <button className="button primary" disabled={busy}>{busy ? '인증 중…' : '로그인'} <ArrowRight size={17} /></button>
        </form>
      </div>
      <footer>Kanvas {version?.version || '확인 중'} · Offline-ready enterprise wiki</footer>
    </section>
  </main>
}

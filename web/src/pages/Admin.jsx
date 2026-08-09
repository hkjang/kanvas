import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { api, formatDate } from '../api'
import {
  Activity, Archive, ArchiveRestore, ArrowRight, Braces, CheckCircle2, ChevronRight,
  CircleAlert, Clock3, Copy, Database, Download, FileSearch, Gauge, HardDrive,
  KeyRound, Layers3, ListChecks, LockKeyhole, Menu, Plus, RefreshCw, ScrollText,
  Search, Server, Settings2, ShieldCheck, Trash2, UserPlus, Users, X, XCircle,
} from 'lucide-react'

const AdminUX = createContext(null)
const emptyOIDCSettings = { enabled: false, issuer: '', clientId: '', clientSecret: '', groupsClaim: 'groups', adminGroup: 'kanvas-admins', autoProvision: true }

const navigation = [
  { label: '운영', items: [['', Gauge, '개요'], ['status', Activity, '서비스 상태'], ['audit', ScrollText, '감사 로그']] },
  { label: '워크스페이스', items: [['users', Users, '사용자'], ['groups', UserPlus, '그룹'], ['spaces', Layers3, 'Space']] },
  { label: '데이터 및 전환', items: [['sources', Database, '데이터 원본'], ['migration', RefreshCw, 'Migration Center']] },
  { label: '플랫폼', items: [['oidc', KeyRound, '인증 및 SSO'], ['security', LockKeyhole, 'API 및 MCP'], ['settings', Settings2, '운영 설정']] },
]

export function Admin({ account, version }) {
  const [toast, setToast] = useState(null)
  const [dialog, setDialog] = useState(null)
  const [navOpen, setNavOpen] = useState(false)
  const notify = useCallback((message, tone = 'success') => {
    const id = Date.now()
    setToast({ id, message, tone })
    window.setTimeout(() => setToast(current => current?.id === id ? null : current), 3600)
  }, [])
  const confirm = useCallback(options => new Promise(resolve => setDialog({ ...options, resolve })), [])
  const closeDialog = value => {
    dialog?.resolve(value)
    setDialog(null)
  }

  return <AdminUX.Provider value={{ notify, confirm }}>
    <div className={`admin-layout ${navOpen ? 'nav-open' : ''}`}>
      <button className="admin-mobile-toggle" onClick={() => setNavOpen(!navOpen)} aria-label="관리 메뉴 열기">{navOpen ? <X /> : <Menu />}</button>
      <aside>
        <div className="admin-brand"><p className="eyebrow">KANVAS CONTROL</p><h2>서비스 관리</h2><small>운영 정책과 전환을 한곳에서 관리합니다.</small></div>
        <nav aria-label="관리 메뉴">{navigation.map(group => <section className="admin-nav-group" key={group.label}><span>{group.label}</span>{group.items.map(([path, Icon, label]) => <NavLink key={path} to={`/admin/${path}`} end={path === ''} onClick={() => setNavOpen(false)}><Icon /><b>{label}</b></NavLink>)}</section>)}</nav>
        <footer><ShieldCheck /><span>Administrator zone<small>Kanvas {version?.version}</small></span></footer>
      </aside>
      <main>
        <Routes>
          <Route index element={<AdminOverview />} />
          <Route path="users" element={<UsersPage account={account} />} />
          <Route path="groups" element={<GroupsPage />} />
          <Route path="spaces" element={<SpacesPage />} />
          <Route path="sources" element={<DataSources />} />
          <Route path="oidc" element={<OIDCSettings />} />
          <Route path="migration" element={<MigrationCenter />} />
          <Route path="security" element={<SecuritySettings />} />
          <Route path="settings" element={<SystemSettings />} />
          <Route path="audit" element={<Audit />} />
          <Route path="status" element={<ServiceStatus />} />
          <Route path="*" element={<Navigate to="/admin" replace />} />
        </Routes>
      </main>
    </div>
    {toast && <div className={`admin-toast ${toast.tone}`} role="status">{toast.tone === 'success' ? <CheckCircle2 /> : <CircleAlert />}<span>{toast.message}</span><button onClick={() => setToast(null)} aria-label="알림 닫기"><X /></button></div>}
    {dialog && <div className="modal-backdrop" role="presentation" onMouseDown={() => closeDialog(false)}><div className="modal compact" role="dialog" aria-modal="true" aria-labelledby="confirm-title" onMouseDown={event => event.stopPropagation()}><span className={`dialog-icon ${dialog.danger ? 'danger' : ''}`}>{dialog.danger ? <CircleAlert /> : <ShieldCheck />}</span><h2 id="confirm-title">{dialog.title}</h2><p>{dialog.message}</p><div className="modal-actions"><button className="button secondary" onClick={() => closeDialog(false)}>취소</button><button className={`button ${dialog.danger ? 'danger' : 'primary'}`} onClick={() => closeDialog(true)}>{dialog.confirmLabel || '확인'}</button></div></div></div>}
  </AdminUX.Provider>
}

function useAdminUX() { return useContext(AdminUX) }

function AdminHeader({ eyebrow, title, description, action }) {
  return <header className="admin-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>{action && <div className="admin-header-actions">{action}</div>}</header>
}

function AdminOverview() {
  const [overview, setOverview] = useState(null)
  const [migration, setMigration] = useState(null)
  const [status, setStatus] = useState(null)
  const [events, setEvents] = useState([])
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    try {
      const [summary, migrationData, serviceData, auditData] = await Promise.all([
        api('/api/v1/admin/overview'), api('/api/v1/admin/migration'), api('/api/v1/admin/status'), api('/api/v1/admin/audit?limit=6'),
      ])
      setOverview(summary); setMigration(migrationData); setStatus(serviceData); setEvents(auditData || []); setError('')
    } catch (loadError) { setError(loadError.message) }
  }, [])
  useEffect(() => { load() }, [load])
  if (!overview && !error) return <PageLoading title="운영 현황을 불러오는 중" />
  if (error && !overview) return <ErrorState message={error} retry={load} />
  const checks = migration?.checks || []
  const pass = checks.filter(check => ['PASS', 'APPROVED'].includes(check.status)).length
  return <>
    <AdminHeader eyebrow="SERVICE OVERVIEW" title="운영 관제판" description="사용자, 콘텐츠, 데이터 전환과 런타임 상태를 한 화면에서 판단합니다." action={<button className="button secondary" onClick={load}><RefreshCw /> 새로고침</button>} />
    <section className="admin-metrics overview-metrics">
      <Metric label="활성 사용자" value={number(overview.activeUsers)} note={`전체 ${number(overview.users)} · 관리자 ${number(overview.administrators)}`} tone="green" icon={Users} />
      <Metric label="운영 Space" value={number(overview.spaces - overview.archivedSpaces)} note={`보관 ${number(overview.archivedSpaces)} · 문서 ${number(overview.pages)}`} tone="blue" icon={Layers3} />
      <Metric label="전환 준비도" value={`${migration?.readiness || 0}%`} note={`${pass}/${checks.length} evidence checks`} tone="amber" icon={Gauge} />
      <Metric label="활성 세션" value={number(overview.activeSessions)} note={`개인 키 ${number(overview.activeApiKeys)} · 감사 24h ${number(overview.auditEvents24h)}`} tone="green" icon={ShieldCheck} />
    </section>
    <section className="admin-grid overview-grid">
      <article className="admin-card readiness-card"><CardTitle title="Migration Readiness" description="Cutover Gate 최신 근거" icon={ListChecks} trailing={<StatusPill status={migration?.phase || 'LOADING'} />} />{checks.length ? checks.slice(0, 7).map(check => <CheckRow key={`${check.category}-${check.name}`} label={`${check.category} · ${check.name}`} status={check.status} />) : <EmptyState title="검증 결과 없음" description="Schema Discovery와 Snapshot을 실행하면 근거가 표시됩니다." />}<NavLink className="text-link" to="/admin/migration">Migration Center 열기 <ArrowRight /></NavLink></article>
      <article className="admin-card"><CardTitle title="운영 신호" description="즉시 확인할 항목" icon={Activity} /><SignalRow label="서비스" value="UP" tone="success" note={status?.service?.version} /><SignalRow label="PostgreSQL" value={status?.database?.status?.toUpperCase() || 'CHECKING'} tone="success" note={`${status?.database?.totalConnections || 0}/${status?.database?.maxConnections || 0} connections`} /><SignalRow label="미해결 콘텐츠" value={number(overview.openExceptions)} tone={overview.openExceptions ? 'warning' : 'success'} note="Unsupported Content" /><SignalRow label="CDC 오류" value={number(migration?.failedEvents)} tone={migration?.failedEvents ? 'danger' : 'success'} note={`${migration?.cdcLagMs || 0} ms lag`} /><NavLink className="text-link" to="/admin/status">서비스 상세 상태 <ArrowRight /></NavLink></article>
      <article className="admin-card span-full"><CardTitle title="최근 관리자 활동" description="가장 최근 감사 이벤트" icon={Clock3} trailing={<NavLink className="subtle-link" to="/admin/audit">전체 보기 <ChevronRight /></NavLink>} /><div className="activity-stream">{events.map(event => <div key={event.id}><span className="activity-dot" /><div><strong>{event.action}</strong><small>{event.actor} · {event.resourceType || 'SYSTEM'} {event.resourceId?.slice(0, 12)}</small></div><time>{relativeTime(event.createdAt)}</time></div>)}{events.length === 0 && <EmptyState title="활동 없음" description="관리 작업이 수행되면 이곳에 기록됩니다." />}</div></article>
    </section>
    <section className="quick-admin-links"><QuickLink to="/admin/users" icon={Users} title="사용자 운영" text="역할·상태 관리" /><QuickLink to="/admin/groups" icon={UserPlus} title="그룹 구성" text="멤버십 관리" /><QuickLink to="/admin/spaces" icon={Layers3} title="Space 인벤토리" text="문서·첨부 현황" /><QuickLink to="/admin/settings" icon={Settings2} title="운영 정책" text="서비스·세션 설정" /></section>
  </>
}

function UsersPage({ account }) {
  const { notify, confirm } = useAdminUX()
  const [users, setUsers] = useState([])
  const [query, setQuery] = useState('')
  const [filters, setFilters] = useState({ role: 'ALL', status: 'ALL', provider: 'ALL' })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busyID, setBusyID] = useState('')
  const load = useCallback(async () => {
    setLoading(true)
    try { setUsers(await api(`/api/v1/admin/users?q=${encodeURIComponent(query.trim())}`) || []); setError('') }
    catch (loadError) { setError(loadError.message) }
    finally { setLoading(false) }
  }, [query])
  useEffect(() => { const timer = window.setTimeout(load, 220); return () => window.clearTimeout(timer) }, [load])
  const visible = useMemo(() => users.filter(user => (filters.role === 'ALL' || user.role === filters.role) && (filters.status === 'ALL' || user.status === filters.status) && (filters.provider === 'ALL' || user.identityProvider === filters.provider)), [users, filters])
  const updateUser = async (user, next) => {
    const destructive = next.status === 'DISABLED' || (user.role === 'ADMIN' && next.role === 'USER')
    if (destructive && !await confirm({ title: '사용자 권한을 변경할까요?', message: next.status === 'DISABLED' ? `${user.displayName}의 모든 세션과 개인 API 키가 즉시 폐기됩니다.` : `${user.displayName}에게서 서비스 관리자 권한을 회수합니다.`, danger: true, confirmLabel: '변경 적용' })) return
    setBusyID(user.id)
    try {
      const updated = await api(`/api/v1/admin/users/${user.id}`, { method: 'PATCH', body: JSON.stringify({ role: next.role, status: next.status }) })
      setUsers(current => current.map(item => item.id === user.id ? updated : item))
      notify(`${updated.displayName}의 계정 정책을 저장했습니다.`)
    } catch (updateError) { notify(updateError.message, 'error') }
    finally { setBusyID('') }
  }
  return <>
    <AdminHeader eyebrow="IDENTITY DIRECTORY" title="사용자 관리" description="OIDC·로컬·마이그레이션 계정의 역할과 접근 상태를 통합 관리합니다." action={<button className="button secondary" onClick={load}><RefreshCw /> 새로고침</button>} />
    <section className="directory-summary"><span><Users /><b>{number(users.length)}</b> 검색 결과</span><span><ShieldCheck /><b>{number(users.filter(user => user.role === 'ADMIN').length)}</b> 관리자</span><span><CircleAlert /><b>{number(users.filter(user => user.status === 'DISABLED').length)}</b> 비활성</span></section>
    <section className="admin-card admin-toolbar"><div className="admin-search"><Search /><input value={query} onChange={event => setQuery(event.target.value)} placeholder="이름, 사용자명, 이메일 검색" aria-label="사용자 검색" />{query && <button onClick={() => setQuery('')} aria-label="검색어 지우기"><X /></button>}</div><select value={filters.role} onChange={event => setFilters({ ...filters, role: event.target.value })} aria-label="역할 필터"><option value="ALL">모든 역할</option><option value="ADMIN">관리자</option><option value="USER">사용자</option></select><select value={filters.status} onChange={event => setFilters({ ...filters, status: event.target.value })} aria-label="상태 필터"><option value="ALL">모든 상태</option><option value="ACTIVE">활성</option><option value="DISABLED">비활성</option></select><select value={filters.provider} onChange={event => setFilters({ ...filters, provider: event.target.value })} aria-label="인증 원본 필터"><option value="ALL">모든 인증 원본</option><option value="LOCAL">LOCAL</option><option value="OIDC">OIDC</option><option value="MIGRATED">MIGRATED</option></select></section>
    <section className="admin-card data-table user-table"><div className="data-table-head"><span>사용자</span><span>인증 원본</span><span>그룹</span><span>최근 로그인</span><span>역할</span><span>상태</span></div>{loading ? <TableLoading columns={6} /> : error ? <ErrorState message={error} retry={load} compact /> : visible.map(user => <div className={`data-row ${user.status === 'DISABLED' ? 'is-muted' : ''}`} key={user.id}><span className="user-cell"><Avatar name={user.displayName} /><span><strong>{user.displayName}{user.id === account?.user?.id && <em>나</em>}</strong><small>{user.email || user.username}</small></span></span><span><StatusPill status={user.identityProvider} /></span><span>{number(user.groupCount)}</span><span>{user.lastLoginAt ? formatDate(user.lastLoginAt) : '로그인 기록 없음'}</span><span><select value={user.role} disabled={busyID === user.id} onChange={event => updateUser(user, { role: event.target.value, status: user.status })}><option value="USER">사용자</option><option value="ADMIN">관리자</option></select></span><span><button className={`status-toggle ${user.status === 'ACTIVE' ? 'active' : ''}`} disabled={busyID === user.id || user.id === account?.user?.id} onClick={() => updateUser(user, { role: user.role, status: user.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE' })}><i />{user.status === 'ACTIVE' ? '활성' : '비활성'}</button></span></div>)}{!loading && !error && visible.length === 0 && <EmptyState title="조건에 맞는 사용자 없음" description="검색어나 필터를 변경해 보세요." />}</section>
    <p className="admin-footnote"><ShieldCheck /> 마지막 활성 관리자는 강등하거나 비활성화할 수 없습니다. OIDC 관리자 역할은 다음 로그인 시 Keycloak 그룹 정책으로 다시 동기화될 수 있습니다.</p>
  </>
}

function GroupsPage() {
  const { notify, confirm } = useAdminUX()
  const [groups, setGroups] = useState([])
  const [users, setUsers] = useState([])
  const [selected, setSelected] = useState(null)
  const [members, setMembers] = useState([])
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ name: '', description: '' })
  const [addUserID, setAddUserID] = useState('')
  const [busy, setBusy] = useState(false)
  const load = useCallback(async () => {
    const [groupData, userData] = await Promise.all([api('/api/v1/admin/groups'), api('/api/v1/admin/users')])
    setGroups(groupData || []); setUsers(userData || [])
    setSelected(current => current && groupData?.some(group => group.id === current.id) ? groupData.find(group => group.id === current.id) : groupData?.[0] || null)
  }, [])
  useEffect(() => { load().catch(error => notify(error.message, 'error')) }, [load, notify])
  const loadMembers = useCallback(async () => { if (selected) setMembers(await api(`/api/v1/admin/groups/${selected.id}/members`) || []) }, [selected])
  useEffect(() => { loadMembers().catch(error => notify(error.message, 'error')) }, [loadMembers, notify])
  const createGroup = async event => {
    event.preventDefault(); setBusy(true)
    try { const group = await api('/api/v1/admin/groups', { method: 'POST', body: JSON.stringify(form) }); setCreateOpen(false); setForm({ name: '', description: '' }); await load(); setSelected(group); notify(`${group.name} 그룹을 만들었습니다.`) }
    catch (error) { notify(error.message, 'error') }
    finally { setBusy(false) }
  }
  const addMember = async event => {
    event.preventDefault(); if (!selected || !addUserID) return; setBusy(true)
    try { await api(`/api/v1/admin/groups/${selected.id}/members`, { method: 'POST', body: JSON.stringify({ userId: addUserID }) }); setAddUserID(''); await Promise.all([loadMembers(), load()]); notify('그룹 멤버를 추가했습니다.') }
    catch (error) { notify(error.message, 'error') }
    finally { setBusy(false) }
  }
  const removeMember = async member => {
    if (!await confirm({ title: '그룹에서 제거할까요?', message: `${member.displayName}에게 부여된 그룹 기반 Wiki 권한이 즉시 변경될 수 있습니다.`, danger: true, confirmLabel: '멤버 제거' })) return
    try { await api(`/api/v1/admin/groups/${selected.id}/members/${member.id}`, { method: 'DELETE' }); await Promise.all([loadMembers(), load()]); notify('그룹 멤버를 제거했습니다.') }
    catch (error) { notify(error.message, 'error') }
  }
  const available = users.filter(user => !members.some(member => member.id === user.id))
  return <>
    <AdminHeader eyebrow="GROUP DIRECTORY" title="그룹 관리" description="Wiki ACL에 사용되는 그룹과 사용자 멤버십을 구성합니다." action={<button className="button primary" onClick={() => setCreateOpen(true)}><Plus /> 새 그룹</button>} />
    <section className="directory-layout"><article className="admin-card group-browser"><CardTitle title="그룹" description={`${groups.length}개 그룹`} icon={UserPlus} /><div className="group-list">{groups.map(group => <button className={selected?.id === group.id ? 'active' : ''} key={group.id} onClick={() => setSelected(group)}><span className="group-avatar">{group.name.slice(0, 2).toUpperCase()}</span><span><strong>{group.name}</strong><small>{group.memberCount}명 · {group.legacySystem || 'KANVAS'}</small></span><ChevronRight /></button>)}{groups.length === 0 && <EmptyState title="그룹 없음" description="첫 번째 Kanvas 그룹을 만들어 보세요." />}</div></article><article className="admin-card group-detail">{selected ? <><CardTitle title={selected.name} description={selected.description || '설명 없음'} icon={Users} trailing={<StatusPill status={selected.legacySystem || 'KANVAS'} />} /><form className="member-add" onSubmit={addMember}><select value={addUserID} onChange={event => setAddUserID(event.target.value)} required><option value="">추가할 사용자 선택</option>{available.map(user => <option key={user.id} value={user.id}>{user.displayName} ({user.username})</option>)}</select><button className="button secondary" disabled={busy || !addUserID}><UserPlus /> 멤버 추가</button></form><div className="member-list">{members.map(member => <div key={member.id}><Avatar name={member.displayName} /><span><strong>{member.displayName}</strong><small>{member.username} · {member.role}</small></span><StatusPill status={member.status} /><button className="icon-action danger" onClick={() => removeMember(member)} aria-label={`${member.displayName} 제거`}><Trash2 /></button></div>)}{members.length === 0 && <EmptyState title="멤버 없음" description="위 선택 상자에서 사용자를 추가하세요." />}</div></> : <EmptyState title="그룹을 선택하세요" description="왼쪽 목록에서 관리할 그룹을 선택합니다." />}</article></section>
    {createOpen && <div className="modal-backdrop" onMouseDown={() => setCreateOpen(false)}><form className="modal" onSubmit={createGroup} onMouseDown={event => event.stopPropagation()}><div className="modal-heading"><span><UserPlus /></span><div><h2>새 그룹 만들기</h2><p>Space 및 Page ACL에 사용할 그룹입니다.</p></div></div><label>그룹 이름<input value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} maxLength={120} required autoFocus placeholder="예: platform-engineers" /></label><label>설명<textarea value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} placeholder="그룹의 책임과 사용 목적" /></label><div className="modal-actions"><button type="button" className="button secondary" onClick={() => setCreateOpen(false)}>취소</button><button className="button primary" disabled={busy}>그룹 만들기</button></div></form></div>}
  </>
}

function SpacesPage() {
  const { notify, confirm } = useAdminUX()
  const [spaces, setSpaces] = useState([])
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const load = useCallback(async () => { setLoading(true); try { setSpaces(await api('/api/v1/admin/spaces') || []) } finally { setLoading(false) } }, [])
  useEffect(() => { load().catch(error => notify(error.message, 'error')) }, [load, notify])
  const updateStatus = async space => {
    const archive = space.status === 'ACTIVE'
    if (!await confirm({ title: archive ? 'Space를 보관할까요?' : 'Space를 다시 활성화할까요?', message: archive ? `${space.name}의 콘텐츠는 유지되며 보관 상태로 표시됩니다.` : `${space.name}을 활성 운영 Space로 복원합니다.`, danger: archive, confirmLabel: archive ? '보관' : '복원' })) return
    try { const updated = await api(`/api/v1/admin/spaces/${space.id}`, { method: 'PATCH', body: JSON.stringify({ status: archive ? 'ARCHIVED' : 'ACTIVE' }) }); setSpaces(current => current.map(item => item.id === updated.id ? updated : item)); notify(`${space.name} 상태를 변경했습니다.`) }
    catch (error) { notify(error.message, 'error') }
  }
  const visible = spaces.filter(space => `${space.key} ${space.name} ${space.description}`.toLowerCase().includes(query.toLowerCase()))
  return <>
    <AdminHeader eyebrow="WORKSPACE INVENTORY" title="Space 운영" description="Space별 콘텐츠 규모와 보관 상태를 확인하고 수명주기를 관리합니다." action={<button className="button secondary" onClick={load}><RefreshCw /> 새로고침</button>} />
    <section className="admin-card admin-toolbar"><div className="admin-search"><Search /><input value={query} onChange={event => setQuery(event.target.value)} placeholder="Space 이름 또는 키 검색" /><span>{visible.length} / {spaces.length}</span></div></section>
    {loading ? <CardGridLoading /> : <section className="space-admin-grid">{visible.map(space => <article className={`admin-card space-admin-card ${space.status === 'ARCHIVED' ? 'archived' : ''}`} key={space.id}><div className="space-card-top"><span className="space-monogram">{space.key.slice(0, 2)}</span><StatusPill status={space.status} /></div><div><small>{space.key}</small><h2>{space.name}</h2><p>{space.description || 'Space 설명이 없습니다.'}</p></div><dl><div><dt>페이지</dt><dd>{number(space.pageCount)}</dd></div><div><dt>첨부</dt><dd>{number(space.attachmentCount)}</dd></div><div><dt>최근 변경</dt><dd>{formatDate(space.updatedAt)}</dd></div></dl><footer><Link className="button secondary" to={`/spaces/${space.id}`}>Space 열기 <ArrowRight /></Link><button className="button ghost" onClick={() => updateStatus(space)}>{space.status === 'ACTIVE' ? <><Archive /> 보관</> : <><ArchiveRestore /> 복원</>}</button></footer></article>)}{visible.length === 0 && <EmptyState title="Space 없음" description="검색 조건을 변경해 보세요." />}</section>}
  </>
}

function DataSources() {
  const { notify } = useAdminUX()
  const [data, setData] = useState(null)
  const [dsn, setDsn] = useState('')
  const [test, setTest] = useState(null)
  const [busy, setBusy] = useState(false)
  const [settings, setSettings] = useState({ attachmentRoot: '/data/confluence/attachments', batchSize: 500, parallelism: 4 })
  const load = useCallback(async () => {
    const payload = await api('/api/v1/admin/settings'); setData(payload)
    const values = settingsObject(payload)
    setSettings({ attachmentRoot: values['migration.attachment_root'] || '/data/confluence/attachments', batchSize: values['migration.batch_size'] || 500, parallelism: values['migration.parallelism'] || 4 })
  }, [])
  useEffect(() => { load().catch(error => notify(error.message, 'error')) }, [load, notify])
  const testPG = async event => { event.preventDefault(); setBusy(true); setTest(null); try { setTest(await api('/api/v1/admin/connections/postgres/test', { method: 'POST', body: JSON.stringify({ dsn }) })) } catch (error) { setTest({ error: error.message }) } finally { setBusy(false) } }
  const save = async event => { event.preventDefault(); setBusy(true); try { for (const [key, value] of Object.entries({ 'migration.attachment_root': settings.attachmentRoot, 'migration.batch_size': Number(settings.batchSize), 'migration.parallelism': Number(settings.parallelism) })) await saveSetting(key, value, 'Migration source setting'); notify('Migration 원본 설정을 저장했습니다.') } catch (error) { notify(error.message, 'error') } finally { setBusy(false) } }
  return <>
    <AdminHeader eyebrow="DATA SOURCES" title="데이터 원본" description="부팅 연결 상태와 마이그레이션 처리 정책을 안전하게 관리합니다." />
    <section className="connection-grid"><Connection title="Kanvas PostgreSQL" type="TARGET · READ/WRITE" configured={data?.environment?.postgres?.configured} fingerprint={data?.environment?.postgres?.fingerprint} /><Connection title="Confluence MySQL" type="SOURCE · SELECT ONLY" configured={data?.environment?.confluence?.configured} fingerprint={data?.environment?.confluence?.fingerprint} /></section>
    <section className="admin-card"><CardTitle title="PostgreSQL 연결 진단" description="입력값은 진단에만 사용하며 저장하거나 로그로 남기지 않습니다." icon={Database} /><form className="inline-form" onSubmit={testPG}><input type="password" value={dsn} onChange={event => setDsn(event.target.value)} placeholder="postgres://user:password@host:5432/kanvas?sslmode=require" required /><button className="button secondary" disabled={busy}>{busy ? '확인 중…' : '연결 테스트'}</button></form>{test && <div className={test.error ? 'error-box' : 'success-box'}>{test.error || `PostgreSQL ${test.version} · Schema CREATE ${test.schemaCreate ? '허용' : '불가'}`}</div>}</section>
    <form className="admin-card form-grid" onSubmit={save}><CardTitle title="Migration 처리 정책" description="DSN을 제외한 운영값은 관리자 설정으로 보관됩니다." icon={Settings2} /><label className="span-2">Confluence Attachment Root<input value={settings.attachmentRoot} onChange={event => setSettings({ ...settings, attachmentRoot: event.target.value })} required /></label><label>Batch Size<input type="number" min="10" max="5000" value={settings.batchSize} onChange={event => setSettings({ ...settings, batchSize: event.target.value })} /></label><label>Parallelism<input type="number" min="1" max="32" value={settings.parallelism} onChange={event => setSettings({ ...settings, parallelism: event.target.value })} /></label><button className="button primary" disabled={busy}>정책 저장</button></form>
  </>
}

function OIDCSettings() {
  const { notify } = useAdminUX()
  const [form, setForm] = useState(emptyOIDCSettings)
  const [hasSecret, setHasSecret] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api('/api/v1/admin/oidc').then(value => { setForm({ ...emptyOIDCSettings, ...value.config }); setHasSecret(value.hasClientSecret) }).catch(error => notify(error.message, 'error')) }, [notify])
  const save = async event => { event.preventDefault(); setBusy(true); try { await api('/api/v1/admin/oidc', { method: 'PUT', body: JSON.stringify({ config: form, keepExistingSecret: !form.clientSecret && hasSecret }) }); setHasSecret(hasSecret || !!form.clientSecret); setForm({ ...form, clientSecret: '' }); notify('Keycloak OIDC 설정을 저장했습니다.') } catch (error) { notify(error.message, 'error') } finally { setBusy(false) } }
  return <>
    <AdminHeader eyebrow="IDENTITY PROVIDER" title="Keycloak SSO / OIDC" description="Issuer, Client ID, Client Secret만으로 Discovery와 자동 프로비저닝을 구성합니다." action={<StatusPill status={form.enabled ? 'ENABLED' : 'DISABLED'} />} />
    <form className="admin-card form-grid" onSubmit={save}><div className="toggle-row span-2"><div><strong>OIDC 로그인 사용</strong><small>로그인 화면에 Keycloak SSO 버튼을 표시합니다.</small></div><input type="checkbox" checked={form.enabled} onChange={event => setForm({ ...form, enabled: event.target.checked })} /></div><label className="span-2">Issuer URL<input value={form.issuer} onChange={event => setForm({ ...form, issuer: event.target.value })} placeholder="https://keycloak.example/realms/company" required={form.enabled} /></label><label>Client ID<input value={form.clientId} onChange={event => setForm({ ...form, clientId: event.target.value })} placeholder="kanvas" required={form.enabled} /></label><label>Client Secret<input type="password" value={form.clientSecret} onChange={event => setForm({ ...form, clientSecret: event.target.value })} placeholder={hasSecret ? '저장됨 · 변경 시에만 입력' : 'Client secret'} /></label><label>Groups Claim<input value={form.groupsClaim} onChange={event => setForm({ ...form, groupsClaim: event.target.value })} /></label><label>관리자 그룹<input value={form.adminGroup} onChange={event => setForm({ ...form, adminGroup: event.target.value })} /></label><div className="toggle-row span-2"><div><strong>최초 로그인 자동 프로비저닝</strong><small>OIDC subject를 영구 사용자 식별자로 사용합니다.</small></div><input type="checkbox" checked={form.autoProvision} onChange={event => setForm({ ...form, autoProvision: event.target.checked })} /></div><button className="button primary" disabled={busy}>OIDC 설정 저장</button></form>
    <section className="admin-card callout"><ShieldCheck /><div><h3>Callback URL</h3><code>{window.location.origin}/api/v1/auth/oidc/callback</code><p>운영 설정의 서비스 기준 URL이 지정되면 해당 주소가 우선 적용됩니다. TLS 프록시는 X-Forwarded-Proto와 X-Forwarded-Host를 전달해야 합니다.</p></div></section>
  </>
}

function MigrationCenter() {
  const { notify, confirm } = useAdminUX()
  const [data, setData] = useState(null)
  const [jobs, setJobs] = useState([])
  const [macros, setMacros] = useState([])
  const [unsupported, setUnsupported] = useState([])
  const [failedItems, setFailedItems] = useState([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback(async () => { const [dashboard, jobList, macroList, unsupportedList] = await Promise.all([api('/api/v1/admin/migration'), api('/api/v1/admin/migration/jobs'), api('/api/v1/admin/migration/macros'), api('/api/v1/admin/migration/unsupported')]); setData(dashboard); setJobs(jobList || []); setMacros(macroList || []); setUnsupported(unsupportedList || []); setError('') }, [])
  useEffect(() => { load().catch(loadError => setError(loadError.message)); const timer = window.setInterval(() => load().catch(() => {}), 2500); return () => window.clearInterval(timer) }, [load])
  const act = async (work, success) => { setBusy(true); setError(''); try { await work(); await load(); if (success) notify(success) } catch (actionError) { setError(actionError.message) } finally { setBusy(false) } }
  const discover = () => act(() => api('/api/v1/admin/migration/discovery', { method: 'POST' }), 'Schema Discovery를 완료했습니다.')
  const snapshot = () => act(() => api('/api/v1/admin/migration/snapshot', { method: 'POST', body: JSON.stringify({ batchSize: 500, includeUsers: true, includeGroups: true, includeSpaces: true, includePages: true, includeComments: true, includeAttachmentMetadata: true, includePermissions: true }) }), 'Initial Snapshot을 시작했습니다.')
  const cancel = async id => { if (await confirm({ title: '실행 중인 Job을 중지할까요?', message: '완료 레코드는 보존되며 나중에 같은 Job을 재개할 수 있습니다.', danger: true, confirmLabel: '중지 요청' })) act(() => api(`/api/v1/admin/migration/jobs/${id}/cancel`, { method: 'POST' }), '중지 요청을 전송했습니다.') }
  const resume = id => act(() => api(`/api/v1/admin/migration/jobs/${id}/resume`, { method: 'POST' }), 'Migration Job을 재개했습니다.')
  const transition = target => act(() => api('/api/v1/admin/migration/transition', { method: 'POST', body: JSON.stringify({ target }) }), `${target} 상태로 전환했습니다.`)
  const inspectFailed = async id => { try { setFailedItems(await api(`/api/v1/admin/migration/jobs/${id}/items?status=FAILED`) || []) } catch (inspectError) { notify(inspectError.message, 'error') } }
  const discovery = data?.latestDiscovery
  const active = jobs.find(job => ['PENDING', 'RUNNING', 'CANCEL_REQUESTED'].includes(job.status))
  return <>
    <AdminHeader eyebrow="CONFLUENCE MIGRATION CENTER" title="안전한 데이터 전환" description="Legacy 원본을 수정하지 않고 Discovery, Initial Snapshot, 정합성 검증을 실행합니다." action={<><button className="button secondary" onClick={discover} disabled={busy || !!active}><FileSearch /> Schema Discovery</button><button className="button primary" onClick={snapshot} disabled={busy || !discovery || !!active}><RefreshCw /> Initial Snapshot</button></>} />
    {error && <div className="error-box" role="alert">{error}</div>}
    <section className="migration-hero"><div><small>CURRENT PHASE</small><strong>{data?.phase || 'LOADING'}</strong><p>Data Source Mode · {data?.sourceMode || '—'}</p></div><div className="readiness-ring"><strong>{data?.readiness || 0}<small>%</small></strong><span>Readiness</span></div><div><small>ACTIVE JOB</small><strong>{active?.currentEntity || active?.status || 'IDLE'}</strong><p>{active ? `${number(active.processedItems)} / ${number(active.totalItems)}` : `${data?.failedEvents || 0} failed events`}</p></div></section>
    {active && <section className="admin-card job-active"><CardTitle title="Snapshot 실행 중" description={`${active.currentEntity || active.status} · 실패 ${active.failedItems}`} icon={RefreshCw} trailing={<button className="button secondary" onClick={() => cancel(active.id)}>중지 요청</button>} /><Progress value={active.processedItems} total={active.totalItems} /><small>{number(active.processedItems)} / {number(active.totalItems)} records</small></section>}
    <section className="admin-grid"><article className="admin-card"><CardTitle title="Source Discovery" description="실제 설치 스키마 기준" icon={Database} />{discovery ? <dl className="detail-list"><div><dt>MySQL</dt><dd>{discovery.databaseVersion}</dd></div><div><dt>Confluence Build</dt><dd>{discovery.confluenceVersion || 'Unknown'}</dd></div><div><dt>Charset / Collation</dt><dd>{discovery.characterSet} / {discovery.collation}</dd></div><div><dt>Tables</dt><dd>{discovery.tables?.length}</dd></div><div><dt>AO / Unknown</dt><dd>{discovery.aoTables} / {discovery.unknownTables}</dd></div><div><dt>Discovered</dt><dd>{formatDate(discovery.createdAt)}</dd></div></dl> : <EmptyState title="Discovery 필요" description="Schema Discovery를 먼저 실행하세요." />}</article><article className="admin-card"><CardTitle title="Core Object Counts" description="Confluence 주요 테이블" icon={FileSearch} />{discovery?.coreCounts ? <div className="count-list">{Object.entries(discovery.coreCounts).map(([key, value]) => <div key={key}><span>{key}</span><strong>{number(value)}</strong></div>)}</div> : <EmptyState title="집계 결과 없음" />}</article></section>
    <section className="admin-card"><CardTitle title="Migration Jobs" description="완료 레코드를 건너뛰는 재시작 가능 Job" icon={RefreshCw} /><div className="job-list">{jobs.map(job => <div key={job.id}><StatusPill status={job.status} /><div><strong>{job.kind} · {job.currentEntity || 'finished'}</strong><small>{formatDate(job.createdAt)} · {number(job.processedItems)}/{number(job.totalItems)} · failed {job.failedItems}</small></div><Progress value={job.processedItems} total={job.totalItems} compact /><div className="row-actions">{job.failedItems > 0 && <button className="button ghost" onClick={() => inspectFailed(job.id)}>오류 보기</button>}{['FAILED', 'INTERRUPTED', 'CANCELLED', 'COMPLETED_WITH_ERRORS'].includes(job.status) && <button className="button secondary" onClick={() => resume(job.id)}>재개</button>}</div></div>)}{jobs.length === 0 && <EmptyState title="Migration Job 없음" />}</div></section>
    <section className="admin-grid"><article className="admin-card"><CardTitle title="Macro Compatibility" description="페이지 기준 변환 지원 현황" icon={Braces} /><div className="compat-list">{macros.map(macro => <div key={macro.name}><strong>{macro.name}</strong><span>{macro.pageCount} pages · {macro.occurrenceCount} uses</span><b className={macro.supportLevel === 'UNSUPPORTED' ? 'warn' : ''}>{macro.supportLevel}</b></div>)}{macros.length === 0 && <EmptyState title="Snapshot 이후 집계됩니다" />}</div></article><article className="admin-card"><CardTitle title="Unsupported Content" description="수동 확인이 필요한 항목" icon={CircleAlert} /><div className="unsupported-list">{unsupported.slice(0, 12).map(item => <div key={item.id} title={item.sample}><span>{item.kind}</span><strong>{item.name}</strong><b>{item.occurrenceCount}</b></div>)}{unsupported.length === 0 && <EmptyState title="열린 예외 없음" />}</div></article></section>
    <section className="admin-card"><CardTitle title="Cutover Gate" description="필수 검사가 PASS 또는 APPROVED여야 전환됩니다." icon={ListChecks} /><div className="checks-grid">{(data?.checks || []).map(check => <CheckRow key={`${check.category}-${check.name}`} label={`${check.category} · ${check.name}`} status={check.status} />)}{!data?.checks?.length && <EmptyState title="검증 엔진 실행 전" description="근거가 없으면 Cutover는 허용되지 않습니다." />}</div><div className="transition-bar"><span>상태 전환</span>{nextStates(data?.phase).map(target => <button key={target} className="button secondary" disabled={busy || !!active} onClick={() => transition(target)}>{target} <ArrowRight /></button>)}</div></section>
    {failedItems.length > 0 && <div className="modal-backdrop" onMouseDown={() => setFailedItems([])}><div className="modal migration-errors" onMouseDown={event => event.stopPropagation()}><div className="modal-heading"><span className="danger"><CircleAlert /></span><div><h2>실패 레코드</h2><p>재개 전 원본 데이터와 오류 원인을 확인하세요.</p></div></div><div className="error-item-list">{failedItems.map(item => <div key={item.id}><strong>{item.entityType} · {item.legacyId}</strong><p>{item.error}</p><small>retry {item.retryCount}</small></div>)}</div><div className="modal-actions"><button className="button secondary" onClick={() => setFailedItems([])}>닫기</button></div></div></div>}
  </>
}

function SecuritySettings() {
  const { notify } = useAdminUX()
  const [overview, setOverview] = useState(null)
  useEffect(() => { api('/api/v1/admin/overview').then(setOverview).catch(error => notify(error.message, 'error')) }, [notify])
  const copy = async value => { await navigator.clipboard.writeText(value); notify('엔드포인트를 복사했습니다.') }
  return <>
    <AdminHeader eyebrow="SECURITY & EXTENSIONS" title="API 및 MCP" description="Kanvas ACL을 그대로 적용하는 사용자별 자동화 인터페이스입니다." />
    <section className="admin-metrics"><Metric label="활성 개인 키" value={number(overview?.activeApiKeys)} note="해시 저장 · 원문 1회 노출" tone="green" icon={KeyRound} /><Metric label="활성 세션" value={number(overview?.activeSessions)} note="비활성화 시 즉시 폐기" tone="blue" icon={ShieldCheck} /><Metric label="MCP 도구" value="8" note="Wiki ACL-aware tools" tone="green" icon={Braces} /><Metric label="OpenAPI" value="3.1" note="서비스 내장 명세" tone="amber" icon={FileSearch} /></section>
    <section className="admin-grid"><EndpointCard icon={KeyRound} title="REST API" description="개인 키로 Wiki API를 호출합니다." endpoint="Authorization: Bearer knv_..." onCopy={copy} /><EndpointCard icon={Braces} title="MCP Server" description="Streamable HTTP JSON-RPC 엔드포인트입니다." endpoint={`${window.location.origin}/mcp`} onCopy={copy} /></section>
    <section className="admin-card"><CardTitle title="보안 기본값" description="서버에서 강제되는 보호 정책" icon={ShieldCheck} /><div className="security-list"><CheckRow label="HttpOnly · SameSite session cookie" status="PASS" /><CheckRow label="CSRF state-changing request guard" status="PASS" /><CheckRow label="CSP · Frame deny · MIME sniffing protection" status="PASS" /><CheckRow label="AES-256-GCM encrypted administrator secrets" status="PASS" /><CheckRow label="Legacy MySQL DML isolation" status="PASS" /><CheckRow label="REST/MCP server-side ACL enforcement" status="PASS" /></div><a className="text-link" href="/api/openapi.yaml" target="_blank" rel="noreferrer">OpenAPI 원문 열기 <ArrowRight /></a></section>
  </>
}

function SystemSettings() {
  const { notify } = useAdminUX()
  const [form, setForm] = useState({ siteName: 'Kanvas', baseURL: '', sessionHours: 12 })
  const [environment, setEnvironment] = useState(null)
  const [busy, setBusy] = useState(false)
  const load = useCallback(async () => { const payload = await api('/api/v1/admin/settings'); const values = settingsObject(payload); setForm({ siteName: values['site.name'] || 'Kanvas', baseURL: values['site.base_url'] || '', sessionHours: values['security.session_hours'] || 12 }); setEnvironment(payload.environment) }, [])
  useEffect(() => { load().catch(error => notify(error.message, 'error')) }, [load, notify])
  const save = async event => { event.preventDefault(); setBusy(true); try { await saveSetting('site.name', form.siteName, 'Login and service display name'); await saveSetting('site.base_url', form.baseURL, 'Canonical service URL for OIDC callback'); await saveSetting('security.session_hours', Number(form.sessionHours), 'Browser session duration'); notify('운영 설정을 저장했습니다. 새 세션부터 정책이 적용됩니다.') } catch (error) { notify(error.message, 'error') } finally { setBusy(false) } }
  return <>
    <AdminHeader eyebrow="SYSTEM POLICY" title="운영 설정" description="환경변수 외 서비스 정책을 관리자 화면에서 관리합니다." />
    <form className="admin-card form-grid" onSubmit={save}><CardTitle title="서비스 기본 정책" description="로그인 표시명, 기준 URL과 세션 수명을 설정합니다." icon={Settings2} /><label>서비스 표시명<input value={form.siteName} onChange={event => setForm({ ...form, siteName: event.target.value })} maxLength={80} required /></label><label>세션 유지 시간<input type="number" min="1" max="168" value={form.sessionHours} onChange={event => setForm({ ...form, sessionHours: event.target.value })} /><small>1~168시간 · 새 로그인 세션부터 적용</small></label><label className="span-2">서비스 기준 URL<input type="url" value={form.baseURL} onChange={event => setForm({ ...form, baseURL: event.target.value })} placeholder="https://wiki.company.internal" /><small>OIDC Callback URL 생성에 사용합니다. 비워두면 Reverse Proxy 헤더를 사용합니다.</small></label><button className="button primary" disabled={busy}>운영 정책 저장</button></form>
    <section className="admin-card"><CardTitle title="부팅 환경변수" description="값은 노출하지 않고 구성 여부와 fingerprint만 표시합니다." icon={Server} /><div className="environment-list"><ConnectionRow label="KANVAS_POSTGRES_DSN" value={environment?.postgres} required /><ConnectionRow label="KANVAS_CONFLUENCE_DSN" value={environment?.confluence} /><ConnectionRow label="KANVAS_BOOTSTRAP_ADMIN" value={{ configured: true }} required /><ConnectionRow label="KANVAS_BOOTSTRAP_ADMIN_PASSWORD" value={{ configured: true }} required /></div></section>
  </>
}

function Audit() {
  const { notify } = useAdminUX()
  const [events, setEvents] = useState([])
  const [query, setQuery] = useState('')
  const [action, setAction] = useState('')
  const [selected, setSelected] = useState(null)
  const [loading, setLoading] = useState(true)
  const load = useCallback(async () => { setLoading(true); try { setEvents(await api(`/api/v1/admin/audit?limit=500&q=${encodeURIComponent(query)}&action=${encodeURIComponent(action)}`) || []) } catch (error) { notify(error.message, 'error') } finally { setLoading(false) } }, [query, action, notify])
  useEffect(() => { const timer = window.setTimeout(load, 180); return () => window.clearTimeout(timer) }, [load])
  const actions = useMemo(() => [...new Set(events.map(event => event.action))].sort(), [events])
  const exportCSV = () => {
    const rows = [['created_at', 'actor', 'action', 'resource_type', 'resource_id', 'remote_addr'], ...events.map(event => [event.createdAt, event.actor, event.action, event.resourceType, event.resourceId, event.remoteAddr])]
    const blob = new Blob([rows.map(row => row.map(csvCell).join(',')).join('\n')], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = `kanvas-audit-${new Date().toISOString().slice(0, 10)}.csv`; link.click(); URL.revokeObjectURL(url); notify(`${events.length}개 감사 이벤트를 내보냈습니다.`)
  }
  return <>
    <AdminHeader eyebrow="AUDIT TRAIL" title="감사 로그" description="인증, 관리자 변경, 문서와 마이그레이션 작업을 추적합니다." action={<button className="button secondary" onClick={exportCSV} disabled={!events.length}><Download /> CSV 내보내기</button>} />
    <section className="admin-card admin-toolbar"><div className="admin-search"><Search /><input value={query} onChange={event => setQuery(event.target.value)} placeholder="행위자, 작업, 리소스 검색" /></div><select value={action} onChange={event => setAction(event.target.value)}><option value="">모든 작업</option>{actions.map(value => <option key={value}>{value}</option>)}</select><button className="button ghost" onClick={load}><RefreshCw /> 새로고침</button></section>
    <section className="admin-card data-table audit-data"><div className="data-table-head"><span>시간</span><span>행위자</span><span>작업</span><span>대상</span><span>Remote</span></div>{loading ? <TableLoading columns={5} /> : events.map(event => <button className="data-row" key={event.id} onClick={() => setSelected(event)}><span>{formatDate(event.createdAt)}</span><span><strong>{event.actor}</strong></span><span><StatusPill status={event.action} /></span><span>{event.resourceType} {event.resourceId?.slice(0, 12)}</span><span>{event.remoteAddr || '—'}</span></button>)}{!loading && events.length === 0 && <EmptyState title="감사 이벤트 없음" description="검색 조건에 해당하는 기록이 없습니다." />}</section>
    {selected && <div className="modal-backdrop" onMouseDown={() => setSelected(null)}><div className="modal audit-detail" onMouseDown={event => event.stopPropagation()}><div className="modal-heading"><span><ScrollText /></span><div><h2>{selected.action}</h2><p>{formatDate(selected.createdAt)} · {selected.actor}</p></div></div><dl className="detail-list"><div><dt>Resource</dt><dd>{selected.resourceType} · {selected.resourceId || '—'}</dd></div><div><dt>Remote</dt><dd>{selected.remoteAddr || '—'}</dd></div></dl><label>Detail<pre>{JSON.stringify(selected.detail || {}, null, 2)}</pre></label><div className="modal-actions"><button className="button secondary" onClick={() => setSelected(null)}>닫기</button></div></div></div>}
  </>
}

function ServiceStatus() {
  const [status, setStatus] = useState(null)
  const [updatedAt, setUpdatedAt] = useState(null)
  const load = useCallback(async () => { const value = await api('/api/v1/admin/status'); setStatus(value); setUpdatedAt(new Date()) }, [])
  useEffect(() => { load(); const timer = window.setInterval(() => load().catch(() => {}), 5000); return () => window.clearInterval(timer) }, [load])
  if (!status) return <PageLoading title="런타임 상태를 확인하는 중" />
  const databaseUsage = status.database.maxConnections ? status.database.totalConnections / status.database.maxConnections * 100 : 0
  return <>
    <AdminHeader eyebrow="OBSERVABILITY" title="서비스 상태" description="5초마다 갱신되는 애플리케이션과 PostgreSQL 런타임 정보입니다." action={<span className="live-indicator"><i /> LIVE · {updatedAt?.toLocaleTimeString('ko-KR')}</span>} />
    <section className="admin-metrics"><Metric label="HTTP Service" value="UP" note={status.service.version} tone="green" icon={Activity} /><Metric label="Uptime" value={duration(status.runtime.uptimeSeconds)} note={`started ${formatDate(status.runtime.startedAt)}`} tone="blue" icon={Clock3} /><Metric label="Goroutines" value={number(status.runtime.goroutines)} note={status.runtime.goVersion} tone="green" icon={Server} /><Metric label="Memory" value={bytes(status.runtime.memoryAllocBytes)} note={`${bytes(status.runtime.memorySystemBytes)} system`} tone="amber" icon={HardDrive} /></section>
    <section className="admin-grid"><article className="admin-card"><CardTitle title="PostgreSQL Connection Pool" description="현재 프로세스 연결 사용량" icon={Database} trailing={<StatusPill status="CONNECTED" />} /><div className="pool-usage"><div><strong>{status.database.totalConnections}</strong><span>/ {status.database.maxConnections} connections</span></div><Progress value={status.database.totalConnections} total={status.database.maxConnections} /><dl><div><dt>Acquired</dt><dd>{status.database.acquiredConnections}</dd></div><div><dt>Idle</dt><dd>{status.database.idleConnections}</dd></div><div><dt>Usage</dt><dd>{databaseUsage.toFixed(1)}%</dd></div></dl></div></article><article className="admin-card"><CardTitle title="Build Information" description="현재 실행 중인 오프라인 이미지" icon={FileSearch} /><dl className="detail-list"><div><dt>Version</dt><dd>{status.service.version}</dd></div><div><dt>Commit</dt><dd><code>{status.service.commit}</code></dd></div><div><dt>Built At</dt><dd>{status.service.builtAt}</dd></div><div><dt>Runtime Time</dt><dd>{formatDate(status.runtime.time)}</dd></div></dl></article></section>
  </>
}

function Metric({ label, value, note, tone, icon: Icon }) { return <article className={tone}>{Icon && <Icon />}<small>{label}</small><strong>{value ?? '—'}</strong><span>{note}</span></article> }
function CardTitle({ title, description, icon: Icon, trailing }) { return <div className="card-title"><div><h2>{title}</h2>{description && <p>{description}</p>}</div>{trailing || (Icon && <Icon />)}</div> }
function StatusPill({ status = '' }) { const normalized = status.toLowerCase(); const success = ['active', 'enabled', 'pass', 'approved', 'complete', 'connected', 'local', 'kanvas'].includes(normalized); const danger = ['failed', 'fail', 'disabled', 'error', 'cancelled'].includes(normalized); const warning = ['warning', 'archived', 'interrupted', 'completed_with_errors', 'migrated'].includes(normalized); return <span className={`state-pill ${success ? 'success' : ''} ${danger ? 'danger' : ''} ${warning ? 'warning' : ''}`}>{status || '—'}</span> }
function CheckRow({ label, status }) { const Icon = status === 'PASS' || status === 'APPROVED' ? CheckCircle2 : status === 'FAIL' ? XCircle : CircleAlert; return <div className={`check-row ${status?.toLowerCase()}`}><Icon /><span>{label}</span><b>{status}</b></div> }
function SignalRow({ label, value, note, tone }) { return <div className="signal-row"><span><i className={tone} />{label}<small>{note}</small></span><strong>{value}</strong></div> }
function QuickLink({ to, icon: Icon, title, text }) { return <NavLink to={to}><span><Icon /></span><div><strong>{title}</strong><small>{text}</small></div><ChevronRight /></NavLink> }
function Connection({ title, type, configured, fingerprint }) { return <article className="connection-card"><span className={configured ? 'connected' : 'disconnected'}><Database /></span><div><small>{type}</small><h2>{title}</h2><p>{configured ? `Connected · ${fingerprint}` : 'Not configured'}</p></div><b>{configured ? 'CONNECTED' : 'REQUIRED'}</b></article> }
function ConnectionRow({ label, value, required }) { return <div><span><Server /><strong>{label}</strong>{required && <em>필수</em>}</span><span>{value?.configured ? <><StatusPill status="CONFIGURED" /><code>{value.fingerprint}</code></> : <StatusPill status={required ? 'REQUIRED' : 'OPTIONAL'} />}</span></div> }
function EndpointCard({ icon: Icon, title, description, endpoint, onCopy }) { return <article className="admin-card endpoint-card"><span><Icon /></span><div><h2>{title}</h2><p>{description}</p><code>{endpoint}</code></div><button className="icon-action" onClick={() => onCopy(endpoint)} aria-label={`${title} 복사`}><Copy /></button></article> }
function Avatar({ name = '' }) { return <span className="admin-avatar">{name.split(/\s+/).map(value => value[0]).join('').slice(0, 2).toUpperCase() || 'K'}</span> }
function Progress({ value = 0, total = 0, compact }) { const percent = Math.min(100, total ? value / total * 100 : 0); return <div className={compact ? 'mini-progress' : 'progress-track'} aria-label={`${percent.toFixed(0)}% 완료`}><i style={{ width: `${percent}%` }} /></div> }
function EmptyState({ title, description }) { return <div className="admin-empty"><FileSearch /><strong>{title}</strong>{description && <p>{description}</p>}</div> }
function ErrorState({ message, retry, compact }) { return <div className={`admin-error-state ${compact ? 'compact' : ''}`}><CircleAlert /><strong>정보를 불러오지 못했습니다.</strong><p>{message}</p>{retry && <button className="button secondary" onClick={retry}>다시 시도</button>}</div> }
function PageLoading({ title }) { return <div className="page-loading"><span className="loading-orbit" /><strong>{title}</strong><p>잠시만 기다려 주세요.</p></div> }
function TableLoading({ columns }) { return <div className="table-loading">{[0, 1, 2, 3].map(row => <div key={row}>{Array.from({ length: columns }).map((_, column) => <i key={column} />)}</div>)}</div> }
function CardGridLoading() { return <section className="space-admin-grid">{[0, 1, 2].map(value => <article className="admin-card card-loading" key={value}><i /><i /><i /></article>)}</section> }
function number(value) { return Number(value || 0).toLocaleString('ko-KR') }
function bytes(value) { const amount = Number(value || 0); if (amount < 1024) return `${amount} B`; if (amount < 1024 ** 2) return `${(amount / 1024).toFixed(1)} KB`; if (amount < 1024 ** 3) return `${(amount / 1024 ** 2).toFixed(1)} MB`; return `${(amount / 1024 ** 3).toFixed(1)} GB` }
function duration(seconds = 0) { const days = Math.floor(seconds / 86400); const hours = Math.floor((seconds % 86400) / 3600); const minutes = Math.floor((seconds % 3600) / 60); return days ? `${days}d ${hours}h` : hours ? `${hours}h ${minutes}m` : `${minutes}m` }
function relativeTime(value) { const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000)); if (seconds < 60) return '방금 전'; if (seconds < 3600) return `${Math.floor(seconds / 60)}분 전`; if (seconds < 86400) return `${Math.floor(seconds / 3600)}시간 전`; return `${Math.floor(seconds / 86400)}일 전` }
function settingsObject(payload) { return Object.fromEntries((payload?.settings || []).map(setting => [setting.key, setting.value])) }
function saveSetting(key, value, description) { return api('/api/v1/admin/settings', { method: 'PUT', body: JSON.stringify({ key, value, description, secret: false }) }) }
function csvCell(value) { const text = String(value ?? ''); return `"${text.replaceAll('"', '""')}"` }
function nextStates(phase) { return ({ LEGACY: ['DISCOVERY'], DISCOVERY: ['LEGACY'], SNAPSHOT: [], CDC_SYNC: ['VERIFY'], VERIFY: ['SHADOW'], SHADOW: ['CUTOVER_READY'], CUTOVER_READY: ['FREEZE', 'LEGACY'], FREEZE: ['FINAL_SYNC', 'WINBACK'], FINAL_SYNC: ['CUTOVER', 'WINBACK'], CUTOVER: ['STABILIZING', 'WINBACK'], STABILIZING: ['COMPLETE', 'WINBACK'], WINBACK: ['LEGACY'], ERROR: ['LEGACY', 'DISCOVERY'] })[phase] || [] }

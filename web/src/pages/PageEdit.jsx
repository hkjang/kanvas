import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'
import { RichEditor } from '../components/RichEditor'
import { ChevronRight, Save, X } from 'lucide-react'

export function PageEdit() {
  const { pageId } = useParams(); const navigate = useNavigate(); const [page, setPage] = useState(null); const [title, setTitle] = useState(''); const [message, setMessage] = useState(''); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const editorRef = useRef(null)
  useEffect(() => { api(`/api/v1/pages/${pageId}`).then(v => { setPage(v); setTitle(v.title) }).catch(e => setError(e.message)) }, [pageId])
  const editorReady = useCallback(editor => { editorRef.current = editor }, [])
  const save = async () => { if (!editorRef.current || !title.trim()) return; setBusy(true); setError(''); try { const updated = await api(`/api/v1/pages/${pageId}`, { method: 'PUT', body: JSON.stringify({ title, editorDocument: editorRef.current.getJSON(), renderedText: editorRef.current.getText({ blockSeparator: '\n' }), changeMessage: message, version: page.currentVersion }) }); navigate(`/pages/${updated.id}`) } catch (e) { setError(e.message) } finally { setBusy(false) } }
  if (!page) return <div className="page-frame">{error || '편집기를 불러오는 중…'}</div>
  return <div className="edit-page"><div className="edit-top"><div className="breadcrumbs"><Link to={`/pages/${page.id}`}>{page.title}</Link><ChevronRight />편집</div><div><Link className="button ghost" to={`/pages/${page.id}`}><X /> 취소</Link><button className="button primary" onClick={save} disabled={busy}><Save /> {busy ? '저장 중…' : '게시'}</button></div></div><div className="edit-canvas"><input className="title-input" value={title} onChange={e => setTitle(e.target.value)} placeholder="페이지 제목" /><RichEditor content={page.editorDocument} onReady={editorReady} /><label className="change-message">변경 설명<input value={message} onChange={e => setMessage(e.target.value)} placeholder="어떤 내용을 바꾸었나요? (선택)" /></label>{error && <div className="error-box">{error}</div>}</div></div>
}

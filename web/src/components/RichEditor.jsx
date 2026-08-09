import React, { useEffect } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import Underline from '@tiptap/extension-underline'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'
import { Bold, Code2, Heading1, Heading2, Italic, List, ListOrdered, Minus, Pilcrow, Quote, Redo2, Table2, Underline as UnderlineIcon, Undo2 } from 'lucide-react'

const extensions = [
  StarterKit,
  Underline,
  Link.configure({ openOnClick: true, autolink: true, protocols: ['http', 'https'] }),
  Placeholder.configure({ placeholder: '아이디어를 문서로 만들어 보세요. / 명령은 곧 제공됩니다.' }),
  Table.configure({ resizable: true }), TableRow, TableHeader, TableCell,
]

export function RichEditor({ content, editable = true, onReady }) {
  const editor = useEditor({ extensions, content: content || { type: 'doc', content: [] }, editable, editorProps: { attributes: { class: 'prose' } } })
  useEffect(() => { if (editor && content) editor.commands.setContent(content) }, [editor, content])
  useEffect(() => { if (editor && onReady) onReady(editor) }, [editor, onReady])
  if (!editor) return <div className="editor-loading">에디터를 불러오는 중…</div>
  if (!editable) return <EditorContent editor={editor} className="viewer-content" />
  const setLink = () => { const previous = editor.getAttributes('link').href || ''; const url = window.prompt('링크 URL', previous); if (url === null) return; if (!url) editor.chain().focus().extendMarkRange('link').unsetLink().run(); else editor.chain().focus().extendMarkRange('link').setLink({ href: url }).run() }
  return <div className="rich-editor"><div className="editor-toolbar">
    <Tool icon={Undo2} label="실행 취소" run={() => editor.chain().focus().undo().run()} disabled={!editor.can().undo()} />
    <Tool icon={Redo2} label="다시 실행" run={() => editor.chain().focus().redo().run()} disabled={!editor.can().redo()} />
    <i />
    <Tool icon={Pilcrow} label="본문" active={editor.isActive('paragraph')} run={() => editor.chain().focus().setParagraph().run()} />
    <Tool icon={Heading1} label="제목 1" active={editor.isActive('heading', { level: 1 })} run={() => editor.chain().focus().toggleHeading({ level: 1 }).run()} />
    <Tool icon={Heading2} label="제목 2" active={editor.isActive('heading', { level: 2 })} run={() => editor.chain().focus().toggleHeading({ level: 2 }).run()} />
    <i />
    <Tool icon={Bold} label="굵게" active={editor.isActive('bold')} run={() => editor.chain().focus().toggleBold().run()} />
    <Tool icon={Italic} label="기울임" active={editor.isActive('italic')} run={() => editor.chain().focus().toggleItalic().run()} />
    <Tool icon={UnderlineIcon} label="밑줄" active={editor.isActive('underline')} run={() => editor.chain().focus().toggleUnderline().run()} />
    <button type="button" title="링크" className={editor.isActive('link') ? 'active' : ''} onClick={setLink}>↗</button>
    <i />
    <Tool icon={List} label="글머리 목록" active={editor.isActive('bulletList')} run={() => editor.chain().focus().toggleBulletList().run()} />
    <Tool icon={ListOrdered} label="번호 목록" active={editor.isActive('orderedList')} run={() => editor.chain().focus().toggleOrderedList().run()} />
    <Tool icon={Quote} label="인용" active={editor.isActive('blockquote')} run={() => editor.chain().focus().toggleBlockquote().run()} />
    <Tool icon={Code2} label="코드 블록" active={editor.isActive('codeBlock')} run={() => editor.chain().focus().toggleCodeBlock().run()} />
    <Tool icon={Table2} label="표" run={() => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()} />
    <Tool icon={Minus} label="구분선" run={() => editor.chain().focus().setHorizontalRule().run()} />
  </div><EditorContent editor={editor} /></div>
}

function Tool({ icon: Icon, label, run, active, disabled }) { return <button type="button" title={label} onClick={run} className={active ? 'active' : ''} disabled={disabled}><Icon /></button> }

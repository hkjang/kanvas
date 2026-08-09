let csrfToken = ''

export function setCSRF(value) {
  csrfToken = value || ''
}

export async function api(path, options = {}) {
  const method = options.method || 'GET'
  const headers = new Headers(options.headers || {})
  if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) headers.set('X-CSRF-Token', csrfToken)
  const response = await fetch(path, { ...options, method, headers, credentials: 'same-origin' })
  if (response.status === 204) return null
  const contentType = response.headers.get('content-type') || ''
  const payload = contentType.includes('json') ? await response.json() : await response.text()
  if (!response.ok) {
    const error = new Error(payload?.error || payload || `요청 실패 (${response.status})`)
    error.status = response.status
    throw error
  }
  return payload
}

export function formatDate(value) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

// Matches the server's same-origin task endpoint contract.
export function isValidTaskEndpointPath(value: string | undefined, fetch: boolean): boolean {
  if (!value) return true
  if (value !== value.trim() || !value.startsWith('/') || value.startsWith('//') || /[#\\\s]/.test(value)) return false
  const count = value.split('{task_id}').length - 1
  if (count !== (fetch ? 1 : 0) || /[{}]/.test(value.replaceAll('{task_id}', ''))) return false
  try {
    const decodedPath = decodeURIComponent(value.split('?')[0])
    decodeURIComponent(value.split('?').slice(1).join('?'))
    return !decodedPath.split('/').some((segment) => segment === '.' || segment === '..')
  } catch {
    return false
  }
}

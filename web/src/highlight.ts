const KEYWORDS = [
  'abstract', 'as', 'async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'def', 'default', 'defer', 'delete', 'do',
  'else', 'enum', 'export', 'extends', 'false', 'finally', 'for', 'from', 'func', 'function', 'go', 'if', 'implements', 'import', 'in',
  'instanceof', 'interface', 'let', 'map', 'new', 'nil', 'null', 'package', 'pass', 'private', 'protected', 'public', 'range', 'return',
  'self', 'static', 'struct', 'super', 'switch', 'this', 'throw', 'true', 'try', 'type', 'typeof', 'var', 'void', 'while', 'with', 'yield'
]

export function extensionOf(name: string) {
  const index = name.lastIndexOf('.')
  return index < 0 ? '' : name.slice(index + 1).toLowerCase()
}

export function escapeHTML(value: string) {
  return value.replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char] as string)
}

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

/**
 * A deliberately small syntax highlighter: it escapes the source first and then
 * wraps strings, comments, numbers and keywords so no markup from the file can
 * ever reach the DOM as HTML.
 */
export function highlight(source: string, extension: string) {
  const escaped = escapeHTML(source)
  const pattern = new RegExp(
    [
      '(&quot;(?:[^&]|&(?!quot;))*&quot;|&#39;(?:[^&]|&(?!#39;))*&#39;|`(?:[^`])*`)',
      '((?:^|\\n)\\s*(?:#|//|--).*?(?=\\n|$)|/\\*[\\s\\S]*?\\*/)',
      '\\b(\\d+(?:\\.\\d+)?)\\b',
      `\\b(${KEYWORDS.map(escapeRegExp).join('|')})\\b`
    ].join('|'),
    'g'
  )
  return escaped.replace(pattern, (match, string, comment, number, keyword) => {
    if (string) return `<span class="tok-string">${match}</span>`
    if (comment) return `<span class="tok-comment">${match}</span>`
    if (number) return `<span class="tok-number">${match}</span>`
    if (keyword) return `<span class="tok-keyword">${match}</span>`
    return match
  })
}

const IMAGE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif']
const AUDIO_EXTENSIONS = ['mp3', 'wav', 'ogg', 'flac', 'm4a', 'opus']
const VIDEO_EXTENSIONS = ['mp4', 'webm', 'ogv', 'mov']
const TEXT_EXTENSIONS = [
  'txt', 'md', 'markdown', 'go', 'json', 'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'css', 'scss', 'xml', 'html', 'htm', 'log',
  'yaml', 'yml', 'toml', 'ini', 'conf', 'csv', 'sh', 'bash', 'py', 'rb', 'java', 'c', 'h', 'cpp', 'hpp', 'rs', 'sql', 'vue'
]

export type PreviewKind = 'image' | 'text' | 'pdf' | 'audio' | 'video' | 'none'

export function previewKind(name: string): PreviewKind {
  const extension = extensionOf(name)
  if (IMAGE_EXTENSIONS.includes(extension)) return 'image'
  if (AUDIO_EXTENSIONS.includes(extension)) return 'audio'
  if (VIDEO_EXTENSIONS.includes(extension)) return 'video'
  if (extension === 'pdf') return 'pdf'
  if (TEXT_EXTENSIONS.includes(extension)) return 'text'
  return 'none'
}

export const TEXT_PREVIEW_LIMIT = 2 * 1024 * 1024

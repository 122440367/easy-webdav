import { describe, expect, it } from 'vitest'
import { escapeHTML, highlight, previewKind } from './highlight'

describe('preview helpers', () => {
  it('classifies previewable types', () => {
    expect(previewKind('photo.PNG')).toBe('image')
    expect(previewKind('clip.webp')).toBe('image')
    expect(previewKind('archive.zip')).toBe('none')
    expect(previewKind('notes.md')).toBe('text')
    expect(previewKind('manual.pdf')).toBe('pdf')
    expect(previewKind('song.mp3')).toBe('audio')
    expect(previewKind('movie.mp4')).toBe('video')
  })

  it('never emits markup coming from the file itself', () => {
    const source = '<script>alert("x")</script>'
    const rendered = highlight(source, 'html')
    expect(rendered).not.toContain('<script>')
    expect(rendered).toContain('&lt;script&gt;')
    expect(escapeHTML(source)).toBe('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;')
  })

  it('marks keywords, strings and numbers', () => {
    const rendered = highlight('const answer = 42\n// note\nconst word = "hi"', 'ts')
    expect(rendered).toContain('tok-keyword')
    expect(rendered).toContain('tok-number')
    expect(rendered).toContain('tok-comment')
    expect(rendered).toContain('tok-string')
  })
})

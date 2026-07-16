import type { editor as monacoEditor } from 'monaco-editor'
import { describe, expect, it, vi } from 'vitest'

import { suppressFindWidgetTooltips } from './monaco-theme'

describe('suppressFindWidgetTooltips', () => {
  it('suppresses find control hovers without blocking clicks', () => {
    const container = document.createElement('div')
    const findWidget = document.createElement('div')
    const previousButton = document.createElement('div')
    findWidget.className = 'find-widget'
    previousButton.className = 'button codicon-find-previous-match'
    previousButton.title = 'Previous Match'
    findWidget.appendChild(previousButton)
    container.appendChild(findWidget)

    const dispose = vi.fn()
    const editor = {
      getContainerDomNode: () => container,
      onDidDispose: vi.fn(() => ({ dispose })),
    } as unknown as monacoEditor.IStandaloneCodeEditor
    const cleanup = suppressFindWidgetTooltips(editor)
    const hover = vi.fn()
    const click = vi.fn()
    previousButton.addEventListener('mouseover', hover)
    previousButton.addEventListener('click', click)

    previousButton.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }))
    previousButton.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(hover).not.toHaveBeenCalled()
    expect(click).toHaveBeenCalledOnce()
    expect(previousButton).not.toHaveAttribute('title')

    cleanup()
    previousButton.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }))
    expect(hover).toHaveBeenCalledOnce()
    expect(dispose).toHaveBeenCalledOnce()
  })
})

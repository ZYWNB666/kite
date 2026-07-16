import { useEffect, useState } from 'react'
import { formatHex } from 'culori'

import { TERMINAL_THEMES } from '@/types/themes'

type MonacoModule = typeof import('monaco-editor')

const definedThemeSignatures = new Map<string, string>()

function getDefaultBackground(actualTheme: 'dark' | 'light') {
  return actualTheme === 'dark' ? '#18181b' : '#ffffff'
}

function defineThemeIfNeeded(
  monaco: MonacoModule,
  themeName: string,
  signature: string,
  definition: Parameters<MonacoModule['editor']['defineTheme']>[1]
) {
  if (definedThemeSignatures.get(themeName) === signature) {
    return
  }

  monaco.editor.defineTheme(themeName, definition)
  definedThemeSignatures.set(themeName, signature)
}

export function getMonacoBackgroundColor(
  cssVariableName: '--background' | '--card',
  actualTheme: 'dark' | 'light'
) {
  const background = getComputedStyle(document.documentElement)
    .getPropertyValue(cssVariableName)
    .trim()

  return formatHex(background) || getDefaultBackground(actualTheme)
}

export function useMonacoBackgroundColor(
  cssVariableName: '--background' | '--card',
  actualTheme: 'dark' | 'light',
  colorTheme?: string
) {
  const [backgroundColor, setBackgroundColor] = useState(() =>
    getMonacoBackgroundColor(cssVariableName, actualTheme)
  )

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      setBackgroundColor(getMonacoBackgroundColor(cssVariableName, actualTheme))
    })

    return () => cancelAnimationFrame(frame)
  }, [cssVariableName, actualTheme, colorTheme])

  return backgroundColor
}

export function defineMonacoBackgroundThemes(
  monaco: MonacoModule,
  {
    darkThemeName,
    lightThemeName,
    backgroundColor,
  }: {
    darkThemeName: string
    lightThemeName: string
    backgroundColor: string
  }
) {
  defineThemeIfNeeded(
    monaco,
    darkThemeName,
    `${darkThemeName}:${backgroundColor}`,
    {
      base: 'vs-dark',
      inherit: true,
      rules: [],
      colors: {
        'editor.background': backgroundColor,
      },
    }
  )

  defineThemeIfNeeded(
    monaco,
    lightThemeName,
    `${lightThemeName}:${backgroundColor}`,
    {
      base: 'vs',
      inherit: true,
      rules: [],
      colors: {
        'editor.background': backgroundColor,
      },
    }
  )
}

export function defineMonacoLogThemes(monaco: MonacoModule) {
  for (const [themeKey, theme] of Object.entries(TERMINAL_THEMES)) {
    defineThemeIfNeeded(
      monaco,
      `log-theme-${themeKey}`,
      `log-theme-${themeKey}`,
      {
        base: themeKey === 'github' ? 'vs' : 'vs-dark',
        inherit: true,
        rules: [{ token: '', foreground: theme.foreground.replace('#', '') }],
        colors: {
          'editor.background': theme.background,
          'editor.foreground': theme.foreground,
          'editorCursor.foreground': theme.cursor,
          'editor.selectionBackground': theme.selection,
          'editor.lineHighlightBackground': theme.selection,
        },
      }
    )
  }
}

/**
 * Keep Monaco's custom find-widget hover from covering its own controls.
 * Monaco listens for `mouseover` on each find action and renders a floating
 * shortcut hint. Near the editor edge that hint can overlap the action, which
 * repeatedly changes the hover target and makes the action impossible to click.
 * Stop only those hover events during capture; click and keyboard events still
 * reach Monaco normally and the existing aria-labels remain available.
 */
export function suppressFindWidgetTooltips(
  editor: import('monaco-editor').editor.IStandaloneCodeEditor
) {
  const containerNode = editor.getContainerDomNode()
  const findControlSelector = [
    '.find-widget .button',
    '.find-widget .monaco-custom-toggle',
    '.monaco-find-peek .button',
    '.monaco-find-peek .monaco-custom-toggle',
  ].join(', ')

  const stopFindControlHover = (event: MouseEvent) => {
    if (!(event.target instanceof Element)) return
    const control = event.target.closest<HTMLElement>(findControlSelector)
    if (!control || !containerNode.contains(control)) return

    // Older Monaco versions may still use a native title tooltip.
    control.removeAttribute('title')
    event.stopImmediatePropagation()
  }

  containerNode.addEventListener('mouseover', stopFindControlHover, true)
  const disposeListener = editor.onDidDispose(() => {
    containerNode.removeEventListener('mouseover', stopFindControlHover, true)
  })

  return () => {
    containerNode.removeEventListener('mouseover', stopFindControlHover, true)
    disposeListener.dispose()
  }
}

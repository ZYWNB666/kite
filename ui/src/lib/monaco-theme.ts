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
 * Fix the Monaco find-widget tooltip flicker.
 *
 * The find widget's navigation buttons (▲▼, etc.) use native `title` attributes
 * for keyboard-shortcut hints. When the widget re-renders during search (which
 * happens on every match update), the browser-native tooltip is destroyed and
 * recreated, causing a rapid flicker when the cursor hovers over those buttons.
 *
 * This function sets up a MutationObserver on the editor's container that
 * strips `title` attributes from all find-widget buttons whenever they appear
 * or change. The keyboard shortcuts still work (F3 / Shift+F3 / Enter / etc.),
 * only the flickering native tooltip is removed.
 *
 * Call this once in the editor's `onMount` callback.
 */
export function suppressFindWidgetTooltips(
  editor: import('monaco-editor').editor.IStandaloneCodeEditor
) {
  const containerNode = editor.getContainerDomNode()

  // Remove titles from all action-label buttons inside the find widget.
  const stripTitles = () => {
    const buttons = containerNode.querySelectorAll<HTMLElement>(
      '.find-widget .button, .find-widget .action-label, .monaco-find-peek .button, .monaco-find-peek .action-label'
    )
    buttons.forEach((btn) => {
      if (btn.title) {
        btn.removeAttribute('title')
      }
    })
  }

  // Strip immediately (in case the widget is already open).
  stripTitles()

  // Observe DOM mutations so we catch the widget whenever it re-renders.
  const observer = new MutationObserver(() => stripTitles())
  observer.observe(containerNode, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['title'],
  })

  // Return a cleanup function.
  return () => observer.disconnect()
}

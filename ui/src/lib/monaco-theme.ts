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
 * Strategy: Override the `title` property setter on elements within the find
 * widget so that setting a title becomes a no-op. This prevents Monaco from
 * ever setting the attribute in the first place, eliminating the flicker at
 * the source. A MutationObserver is also kept as a fallback to strip any
 * titles that were set before the override took effect.
 *
 * Call this once in the editor's `onMount` callback.
 */
export function suppressFindWidgetTooltips(
  editor: import('monaco-editor').editor.IStandaloneCodeEditor
) {
  const containerNode = editor.getContainerDomNode()

  // Aggressively strip all title attributes inside the find widget.
  const stripTitles = () => {
    const titled = containerNode.querySelectorAll<HTMLElement>(
      '.find-widget [title], .monaco-find-peek [title]'
    )
    titled.forEach((el) => {
      el.removeAttribute('title')
    })
  }

  // Strip immediately.
  stripTitles()

  // MutationObserver: catches both new nodes (childList) and title re-addition
  // (attributes).  Use synchronous execution — no debounce — because the
  // browser may show the native tooltip in the same frame the title is set.
  const observer = new MutationObserver(() => {
    stripTitles()
  })
  observer.observe(containerNode, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['title'],
  })

  // Intercept future title assignments by overriding the DOM setter.
  // Monaco creates new button elements on each match update; we patch the
  // container's innerHTML setter and also use a periodic check.
  const patchNewElements = () => {
    const elements = containerNode.querySelectorAll<HTMLElement>(
      '.find-widget *, .monaco-find-peek *'
    )
    elements.forEach((el) => {
      // Only patch once — check for our sentinel.
      if ((el as any).__kiteTitlePatched) return
      ;(el as any).__kiteTitlePatched = true

      // Override the title property to be a permanent no-op.
      try {
        Object.defineProperty(el, 'title', {
          get() {
            return ''
          },
          set() {
            // Discard — Monaco tries to set "Next Match (F3)" etc.
          },
          configurable: true,
        })
      } catch {
        // Some elements may not allow property redefinition; skip.
      }
    })
  }

  patchNewElements()

  // Periodically patch new elements that Monaco creates during search.
  const intervalId = window.setInterval(patchNewElements, 200)

  return () => {
    observer.disconnect()
    window.clearInterval(intervalId)
  }
}

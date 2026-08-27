import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from 'react'
import { Search, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

interface SubmittedSearchInputProps {
  value: string
  onSearch: (value: string) => void
  placeholder: string
  className?: string
  inputClassName?: string
}

export function SubmittedSearchInput({
  value,
  onSearch,
  placeholder,
  className,
  inputClassName,
}: SubmittedSearchInputProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  const isComposing = useRef(false)
  const normalizedDraft = draft.trim()
  const hasChanges = normalizedDraft !== value

  useEffect(() => {
    setDraft(value)
  }, [value])

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (isComposing.current || !hasChanges) return
    setDraft(normalizedDraft)
    onSearch(normalizedDraft)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (
      event.key === 'Enter' &&
      (isComposing.current || event.nativeEvent.isComposing)
    ) {
      event.preventDefault()
    }
  }

  const handleClear = () => {
    setDraft('')
    if (value) {
      onSearch('')
    }
  }

  return (
    <form
      role="search"
      className={cn('flex items-center gap-2', className)}
      onSubmit={handleSubmit}
    >
      <div className="relative min-w-0 flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={draft}
          placeholder={placeholder}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => {
            isComposing.current = true
          }}
          onCompositionEnd={() => {
            isComposing.current = false
          }}
          className={cn('pl-9 pr-9', inputClassName)}
        />
        {(draft || value) && (
          <button
            type="button"
            aria-label="Clear search"
            className="absolute right-2 top-1/2 -translate-y-1/2 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={handleClear}
          >
            <XCircle className="h-4 w-4" />
          </button>
        )}
      </div>
      <Button type="submit" variant="outline" disabled={!hasChanges}>
        {t('common.search')}
      </Button>
    </form>
  )
}

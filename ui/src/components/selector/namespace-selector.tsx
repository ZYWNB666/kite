import { useMemo, useState } from 'react'
import { Namespace } from 'kubernetes-types/core/v1'
import { Check, ChevronsUpDown, Loader2, X } from 'lucide-react'

import { useResources } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

const ALL = '_all'

export function NamespaceSelector({
  selectedNamespaces,
  handleNamespaceChange,
  showAll = false,
  /** Single-select mode (e.g. deployment-create-dialog). When true, behaves
   * like the old single-select combobox: picking an item closes the popover. */
  singleSelect = false,
}: {
  selectedNamespaces: string[]
  handleNamespaceChange: (namespaces: string[]) => void
  showAll?: boolean
  singleSelect?: boolean
}) {
  const [open, setOpen] = useState(false)
  const { data, isLoading } = useResources('namespaces')

  const sortedNamespaces = useMemo(() => {
    if (!data) return []
    return [...data].sort((a, b) => {
      const nameA = a.metadata?.name?.toLowerCase() || ''
      const nameB = b.metadata?.name?.toLowerCase() || ''
      return nameA.localeCompare(nameB)
    })
  }, [data])

  const selectedSet = useMemo(
    () => new Set(selectedNamespaces),
    [selectedNamespaces]
  )

  const isAllSelected = selectedSet.has(ALL)

  const toggle = (ns: string) => {
    if (singleSelect) {
      handleNamespaceChange([ns])
      setOpen(false)
      return
    }
    if (ns === ALL) {
      // Selecting "All" replaces everything
      handleNamespaceChange([ALL])
      return
    }
    // Toggle individual namespace
    if (isAllSelected) {
      // Switching from _all to specific: start fresh with just this one
      handleNamespaceChange([ns])
      return
    }
    const next = selectedSet.has(ns)
      ? selectedNamespaces.filter((n) => n !== ns)
      : [...selectedNamespaces, ns]
    // Never allow empty — fall back to default
    handleNamespaceChange(next.length === 0 ? ['default'] : next)
  }

  const removeNamespace = (e: React.MouseEvent, ns: string) => {
    e.stopPropagation()
    if (ns === ALL) {
      handleNamespaceChange(['default'])
      return
    }
    const next = selectedNamespaces.filter((n) => n !== ns)
    handleNamespaceChange(next.length === 0 ? ['default'] : next)
  }

  // Trigger button label
  const triggerLabel = useMemo(() => {
    if (selectedNamespaces.length === 0) return 'Select namespace...'
    if (isAllSelected) return 'All Namespaces'
    if (selectedNamespaces.length === 1) return selectedNamespaces[0]
    return `${selectedNamespaces.length} namespaces`
  }, [selectedNamespaces, isAllSelected])

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="h-auto min-h-9 w-full min-w-0 justify-between sm:w-auto sm:min-w-[9rem] sm:max-w-[16rem]"
        >
          <span className="flex flex-1 items-center gap-1 overflow-hidden">
            {selectedNamespaces.length > 0 &&
            !isAllSelected &&
            !singleSelect ? (
              <>
                {selectedNamespaces.slice(0, 2).map((ns) => (
                  <span
                    key={ns}
                    className="inline-flex items-center gap-0.5 rounded bg-muted px-1.5 py-0.5 text-xs"
                  >
                    <span className="truncate max-w-[6rem]">{ns}</span>
                    <span
                      role="button"
                      tabIndex={-1}
                      className="pointer-events-auto flex shrink-0 cursor-pointer opacity-60 hover:opacity-100"
                      onClick={(e) => removeNamespace(e, ns)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          removeNamespace(e as unknown as React.MouseEvent, ns)
                        }
                      }}
                    >
                      <X className="h-3 w-3" />
                    </span>
                  </span>
                ))}
                {selectedNamespaces.length > 2 && (
                  <span className="text-xs text-muted-foreground">
                    +{selectedNamespaces.length - 2}
                  </span>
                )}
              </>
            ) : (
              <span className="truncate">{triggerLabel}</span>
            )}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent
        className="w-[max(var(--radix-popover-trigger-width),14rem)] max-w-[calc(100vw-1rem)] p-0"
        align="start"
      >
        <Command>
          <CommandInput placeholder="Search..." className="h-9" />
          <CommandList className="max-h-[300px] overflow-x-hidden overflow-y-auto [ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {isLoading ? (
              <div className="flex items-center justify-center p-6 text-sm">
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                Loading...
              </div>
            ) : (
              <>
                <CommandEmpty>No results.</CommandEmpty>
                <CommandGroup>
                  {showAll && (
                    <CommandItem
                      value="_all All Namespaces"
                      onSelect={() => toggle(ALL)}
                      className="flex items-center"
                    >
                      <CheckIcon checked={isAllSelected} />
                      <span className="truncate font-medium">
                        All Namespaces
                      </span>
                    </CommandItem>
                  )}

                  {sortedNamespaces.map((ns: Namespace) => {
                    const name = ns.metadata?.name || ''
                    const checked = selectedSet.has(name)
                    return (
                      <CommandItem
                        key={name}
                        value={name}
                        onSelect={() => toggle(name)}
                        className="flex items-center"
                      >
                        <CheckIcon checked={checked} />
                        <span
                          className="truncate flex-1 min-w-0"
                          title={name}
                        >
                          {name}
                        </span>
                      </CommandItem>
                    )
                  })}
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
        {!singleSelect && (
          <div className="flex items-center justify-between border-t px-2 py-1.5">
            <span className="text-xs text-muted-foreground">
              {isAllSelected
                ? 'All namespaces selected'
                : `${selectedNamespaces.length} selected`}
            </span>
            <Button
              size="sm"
              variant="ghost"
              className="h-7 px-2 text-xs"
              onClick={() => setOpen(false)}
            >
              Done
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

function CheckIcon({ checked }: { checked: boolean }) {
  return (
    <div
      className={cn(
        'mr-2 flex h-4 w-4 shrink-0 items-center justify-center rounded border',
        checked
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-input'
      )}
    >
      {checked && <Check className="h-3 w-3" strokeWidth={3} />}
    </div>
  )
}

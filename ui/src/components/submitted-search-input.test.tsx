import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { SubmittedSearchInput } from './submitted-search-input'

describe('SubmittedSearchInput', () => {
  it('keeps typing local until Enter submits the search', async () => {
    const user = userEvent.setup()
    const onSearch = vi.fn()
    render(
      <SubmittedSearchInput
        value=""
        onSearch={onSearch}
        placeholder="Search users..."
      />
    )

    const input = screen.getByPlaceholderText('Search users...')
    await user.type(input, 'alice')
    expect(onSearch).not.toHaveBeenCalled()

    await user.keyboard('{Enter}')
    expect(onSearch).toHaveBeenCalledOnce()
    expect(onSearch).toHaveBeenCalledWith('alice')
  })

  it('submits from the button and clears an applied search immediately', async () => {
    const user = userEvent.setup()
    const onSearch = vi.fn()
    const { rerender } = render(
      <SubmittedSearchInput
        value=""
        onSearch={onSearch}
        placeholder="Search audit logs..."
      />
    )

    await user.type(
      screen.getByPlaceholderText('Search audit logs...'),
      'deployment'
    )
    await user.click(screen.getByRole('button', { name: 'common.search' }))
    expect(onSearch).toHaveBeenLastCalledWith('deployment')

    rerender(
      <SubmittedSearchInput
        value="deployment"
        onSearch={onSearch}
        placeholder="Search audit logs..."
      />
    )
    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(onSearch).toHaveBeenLastCalledWith('')
  })

  it('does not submit Enter while an IME composition is active', () => {
    const onSearch = vi.fn()
    render(
      <SubmittedSearchInput
        value=""
        onSearch={onSearch}
        placeholder="Search users..."
      />
    )

    const input = screen.getByPlaceholderText('Search users...')
    fireEvent.compositionStart(input)
    fireEvent.change(input, { target: { value: '用户' } })
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    expect(onSearch).not.toHaveBeenCalled()

    fireEvent.compositionEnd(input)
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.submit(screen.getByRole('search'))
    expect(onSearch).toHaveBeenCalledWith('用户')
  })

  it('restores the draft when URL-backed value changes', () => {
    const { rerender } = render(
      <SubmittedSearchInput
        value="alice"
        onSearch={vi.fn()}
        placeholder="Search users..."
      />
    )

    rerender(
      <SubmittedSearchInput
        value="bob"
        onSearch={vi.fn()}
        placeholder="Search users..."
      />
    )
    expect(screen.getByPlaceholderText('Search users...')).toHaveValue('bob')
  })
})

import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { Dialog } from './Dialog'
import { Form } from './Form'

function Harness() {
  const [open, setOpen] = useState(true)
  const [renders, setRenders] = useState(0)
  return (
    <>
      <button onClick={() => setRenders(renders + 1)}>Rerender</button>
      {/* A new onClose on every render, like the pages that open dialogs. */}
      <Dialog open={open} onClose={() => setOpen(false)} title="Edit">
        <Form onSubmit={() => {}} onCancel={() => setOpen(false)} submitLabel="Save">
          <label>
            Name
            <input defaultValue="" />
          </label>
        </Form>
      </Dialog>
    </>
  )
}

describe('Dialog unsaved-changes guard', () => {
  it('still asks after the parent re-renders with a new onClose', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.type(screen.getByLabelText('Name'), 'Anna')
    await user.click(screen.getByRole('button', { name: 'Rerender' }))
    fireEvent(screen.getByRole('dialog', { name: 'Edit' }), new Event('cancel', { cancelable: true }))
    expect(await screen.findByRole('dialog', { name: 'Discard changes?' })).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Anna')
  })
})

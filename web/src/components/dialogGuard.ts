import { createContext, useContext } from 'react'

export interface DialogGuard {
  /** Registers a check for unsaved input; returns the unregister function. */
  register: (isDirty: () => boolean) => () => void
  /** Closes the dialog, asking first when there are unsaved changes. */
  requestClose: () => void
}

export const GuardContext = createContext<DialogGuard | null>(null)

/** The unsaved-changes guard of the enclosing Dialog, if any. */
export function useDialogGuard() {
  return useContext(GuardContext)
}

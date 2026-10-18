import { createContext, useContext } from 'react'

export interface UploadUi {
  openPicker: () => void
  openExamples: () => void
  openPanel: () => void
  busy: boolean
}

export const UploadUiContext = createContext<UploadUi | null>(null)

export function useUploadUi() {
  const value = useContext(UploadUiContext)
  return (
    value ?? {
      openPicker: () => undefined,
      openExamples: () => undefined,
      openPanel: () => undefined,
      busy: false,
    }
  )
}

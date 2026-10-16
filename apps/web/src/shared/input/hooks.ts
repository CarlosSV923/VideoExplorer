import { use, useEffect, useRef, type RefObject } from 'react'

import { InputContext, type Action, type Handler, type InputApi, isTextEntry } from './context'

function useInput(): InputApi {
  const input = use(InputContext)
  if (!input) throw new Error('useInput must be used inside InputProvider')
  return input
}
export function useGamepadConnected() {
  return useInput().gamepad
}
export function useAction(action: Action, handler: Handler, enabled = true) {
  const { subscribe } = useInput()
  const latest = useRef(handler)
  useEffect(() => {
    latest.current = handler
  }, [handler])
  useEffect(
    () => (enabled ? subscribe(action, (direction) => latest.current(direction)) : undefined),
    [action, enabled, subscribe],
  )
}
export function useDispatch() {
  return useInput().dispatch
}

export function useLetterShortcuts(
  ref: RefObject<HTMLElement | null>,
  shortcuts: Readonly<Record<string, () => void>>,
  enabled = true,
) {
  const latest = useRef(shortcuts)
  useEffect(() => {
    latest.current = shortcuts
  }, [shortcuts])
  useEffect(() => {
    if (!enabled) return
    const onKey = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.altKey ||
        event.ctrlKey ||
        event.metaKey ||
        isTextEntry(event.target as Element | null)
      )
        return
      if (!ref.current?.contains(event.target as Node)) return
      const action = latest.current[event.key.toLowerCase()]
      if (!action || event.key.length !== 1) return
      event.preventDefault()
      action()
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [enabled, ref])
}

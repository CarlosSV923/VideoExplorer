import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import { InputContext, ownsArrows, type Action, type Handler, type InputApi } from './context'
import { GamepadReader, type PadButton, type PadEvent, type PadState } from './gamepad'
import { moveFocus, type Direction } from './spatial'

const padActions: Record<PadButton, Action> = {
  A: 'confirm',
  B: 'back',
  X: 'menu',
  Y: 'search',
  RT: 'upload',
  MENU: 'menu',
  LB: 'prev',
  RB: 'next',
}
const arrowKeys: Record<string, Direction> = {
  ArrowUp: 'up',
  ArrowDown: 'down',
  ArrowLeft: 'left',
  ArrowRight: 'right',
}

function connectedPads(): (PadState | null)[] {
  return typeof navigator.getGamepads === 'function' ? Array.from(navigator.getGamepads()) : []
}
function fallback(action: Action, direction?: Direction) {
  if (action === 'navigate' && direction) return moveFocus(direction)
  if (action === 'confirm') {
    const active = document.activeElement
    if (active instanceof HTMLElement && active !== document.body) {
      active.click()
      return true
    }
  }
  if (action === 'search') {
    const field = document.querySelector<HTMLElement>('[data-shortcut="search"]')
    field?.focus()
    return field !== null
  }
  return false
}

export function InputProvider({ children }: { children: ReactNode }) {
  const handlers = useRef(new Map<Action, Handler[]>())
  const [gamepad, setGamepad] = useState(() => connectedPads().some(Boolean))
  const api = useMemo<InputApi>(
    () => ({
      gamepad,
      dispatch: (action, direction) => {
        const list = handlers.current.get(action) ?? []
        for (let index = list.length - 1; index >= 0; index -= 1)
          if (list[index]?.(direction) !== false) return true
        return fallback(action, direction)
      },
      subscribe: (action, handler) => {
        const list = handlers.current.get(action) ?? []
        handlers.current.set(action, [...list, handler])
        return () =>
          handlers.current.set(
            action,
            (handlers.current.get(action) ?? []).filter((item) => item !== handler),
          )
      },
    }),
    [gamepad],
  )

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
      let handled = false
      if (event.key === 'Escape') handled = api.dispatch('back')
      else if (event.key === '/' && !ownsArrows(document.activeElement))
        handled = api.dispatch('search')
      else if (event.key in arrowKeys && !ownsArrows(document.activeElement))
        handled = api.dispatch('navigate', arrowKeys[event.key])
      if (handled) event.preventDefault()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [api])

  useEffect(() => {
    const update = () => {
      setGamepad(connectedPads().some(Boolean))
    }
    window.addEventListener('gamepadconnected', update)
    window.addEventListener('gamepaddisconnected', update)
    return () => {
      window.removeEventListener('gamepadconnected', update)
      window.removeEventListener('gamepaddisconnected', update)
    }
  }, [])

  useEffect(() => {
    if (!gamepad) return
    const reader = new GamepadReader()
    let frame = 0
    const tick = (now: number) => {
      for (const event of reader.poll(connectedPads(), now)) handlePad(api, event)
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [api, gamepad])
  return <InputContext value={api}>{children}</InputContext>
}

function handlePad(api: InputApi, event: PadEvent) {
  if (event.type === 'move') api.dispatch('navigate', event.direction)
  else api.dispatch(padActions[event.button])
}

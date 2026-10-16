import { createContext } from 'react'

import type { Direction } from './spatial'

export type Action =
  'confirm' | 'back' | 'menu' | 'search' | 'navigate' | 'prev' | 'next' | 'upload'
export type Handler = (direction?: Direction) => boolean | undefined

export interface InputApi {
  gamepad: boolean
  subscribe: (action: Action, handler: Handler) => () => void
  dispatch: (action: Action, direction?: Direction) => boolean
}

export const InputContext = createContext<InputApi | null>(null)

const textInputs = new Set(['text', 'search', 'email', 'url', 'tel', 'password', 'number'])

export function ownsArrows(element: Element | null): boolean {
  if (!element) return false
  if (element instanceof HTMLInputElement) return textInputs.has(element.type)
  return (
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement ||
    (element as HTMLElement).isContentEditable
  )
}

export function isTextEntry(element: Element | null): boolean {
  return ownsArrows(element) || element?.matches('[role="textbox"]') === true
}

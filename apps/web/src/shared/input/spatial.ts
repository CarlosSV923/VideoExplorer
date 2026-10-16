export type Direction = 'up' | 'down' | 'left' | 'right'
export interface Box {
  left: number
  top: number
  right: number
  bottom: number
}

export function pickNext(from: Box, candidates: readonly Box[], direction: Direction): number {
  const centerX = (from.left + from.right) / 2
  const centerY = (from.top + from.bottom) / 2
  let best = -1
  let score = Infinity
  candidates.forEach((candidate, index) => {
    let ahead = 0
    let sideways = 0
    if (direction === 'right') {
      if (candidate.left < centerX || candidate.right <= from.right) return
      ahead = Math.max(0, candidate.left - from.right)
      sideways = gap(from.top, from.bottom, candidate.top, candidate.bottom)
    }
    if (direction === 'left') {
      if (candidate.right > centerX || candidate.left >= from.left) return
      ahead = Math.max(0, from.left - candidate.right)
      sideways = gap(from.top, from.bottom, candidate.top, candidate.bottom)
    }
    if (direction === 'down') {
      if (candidate.top < centerY || candidate.bottom <= from.bottom) return
      ahead = Math.max(0, candidate.top - from.bottom)
      sideways = gap(from.left, from.right, candidate.left, candidate.right)
    }
    if (direction === 'up') {
      if (candidate.bottom > centerY || candidate.top >= from.top) return
      ahead = Math.max(0, from.top - candidate.bottom)
      sideways = gap(from.left, from.right, candidate.left, candidate.right)
    }
    const candidateScore = ahead + sideways * 2
    if (candidateScore < score) {
      score = candidateScore
      best = index
    }
  })
  return best
}

function gap(a1: number, a2: number, b1: number, b2: number) {
  if (b2 < a1) return a1 - b2
  if (b1 > a2) return b1 - a2
  return 0
}

const selector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')
export function tabbables(root: ParentNode): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(selector)).filter(
    (element) => !element.closest('[inert], [aria-hidden="true"]'),
  )
}

function visualElement(element: HTMLElement) {
  return element instanceof HTMLInputElement &&
    element.offsetWidth <= 1 &&
    element.offsetHeight <= 1
    ? (element.closest('label') ?? element)
    : element
}
export function focusables(root: ParentNode) {
  return tabbables(root)
    .map((element) => ({ el: element, box: visualElement(element).getBoundingClientRect() }))
    .filter(({ box }) => box.width > 0 || box.height > 0)
}
export function navigationRoot(doc: Document = document): ParentNode {
  const last = Array.from(doc.querySelectorAll('[aria-modal="true"]')).at(-1)
  return last ?? doc.body
}
export function moveFocus(direction: Direction, doc: Document = document) {
  const items = focusables(navigationRoot(doc))
  const current = doc.activeElement instanceof HTMLElement ? doc.activeElement : null
  const index = current ? items.findIndex((item) => item.el === current) : -1
  if (index < 0) {
    items[0]?.el.focus()
    return items.length > 0
  }
  const from = items[index]
  if (!from) return false
  const others = items.filter((_, itemIndex) => itemIndex !== index)
  const target =
    others[
      pickNext(
        from.box,
        others.map((item) => item.box),
        direction,
      )
    ]
  if (!target) return false
  target.el.focus()
  try {
    target.el.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  } catch {
    /* jsdom does not implement scrolling */
  }
  return true
}

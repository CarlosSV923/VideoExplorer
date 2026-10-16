import { pickNext, type Box } from './spatial'

const box = (left: number, top: number, width = 100, height = 40): Box => ({
  left,
  top,
  right: left + width,
  bottom: top + height,
})

describe('pickNext', () => {
  it('chooses aligned neighbours and changes rows', () => {
    const first = box(0, 0)
    const second = box(120, 0)
    const third = box(240, 0)
    const lower = box(0, 100, 340)
    expect(pickNext(first, [second, third, lower], 'right')).toBe(0)
    expect(pickNext(second, [first, third, lower], 'down')).toBe(2)
    expect(pickNext(lower, [first, second, third], 'up')).toBe(0)
  })

  it('returns -1 when there is no candidate ahead', () => {
    const first = box(0, 0)
    expect(pickNext(first, [box(120, 0)], 'up')).toBe(-1)
  })
})

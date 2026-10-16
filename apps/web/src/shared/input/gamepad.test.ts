import { GamepadReader, repeatDelay, repeatInterval, type PadState } from './gamepad'

function pad(pressed: number[] = [], axes: number[] = [0, 0]): PadState {
  return {
    buttons: Array.from({ length: 17 }, (_, index) => ({ pressed: pressed.includes(index) })),
    axes,
  }
}

describe('GamepadReader', () => {
  it('fires buttons once and repeats movement after the configured delay', () => {
    const reader = new GamepadReader()
    expect(reader.poll([pad([0])], 0)).toEqual([{ type: 'button', button: 'A' }])
    expect(reader.poll([pad([0])], 10)).toEqual([])
    const right = pad([15])
    expect(reader.poll([right], 20)).toEqual([{ type: 'move', direction: 'right' }])
    expect(reader.poll([right], 20 + repeatDelay - 1)).toEqual([])
    expect(reader.poll([right], 20 + repeatDelay)).toEqual([{ type: 'move', direction: 'right' }])
    expect(reader.poll([right], 20 + repeatDelay + repeatInterval)).toHaveLength(1)
  })

  it('applies the dead zone to the left stick', () => {
    const reader = new GamepadReader()
    expect(reader.poll([pad([], [0.3, 0.2])], 0)).toEqual([])
    expect(reader.poll([pad([], [-0.8, 0.2])], 1)).toEqual([{ type: 'move', direction: 'left' }])
  })
})

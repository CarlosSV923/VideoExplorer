import { cx } from './cx'

export type ButtonVariant = 'primary' | 'secondary' | 'outline-accent' | 'danger' | 'danger-solid'
export type ButtonSize = 'lg' | 'md' | 'sm'

const sizes: Record<ButtonSize, string> = {
  lg: 'min-h-control-lg px-7 text-body-lg',
  md: 'min-h-control-md px-5 text-body',
  sm: 'min-h-control-sm px-4 text-body-sm',
}
const variants: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-on-accent hover:bg-accent-hover',
  secondary: 'border border-control bg-transparent text-ink-1 hover:bg-hover',
  'outline-accent': 'border border-accent bg-transparent text-accent hover:bg-hover',
  danger: 'border border-danger bg-transparent text-danger hover:bg-hover',
  'danger-solid': 'bg-danger text-on-danger hover:bg-danger',
}

export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md') {
  return cx(
    'inline-flex items-center justify-center gap-2.5 rounded-pill box-border whitespace-nowrap select-none no-underline transition duration-120 ease-out active:scale-[.98]',
    sizes[size],
    variants[variant],
  )
}

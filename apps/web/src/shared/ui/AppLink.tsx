import { Link, useRouter } from '@tanstack/react-router'
import type { AnchorHTMLAttributes, ComponentProps, ReactNode } from 'react'

export interface AppLinkProps extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> {
  href: string
  children: ReactNode
}

/** Accessible in-app anchor that keeps TanStack Router navigation client-side. */
export function AppLink({ href, children, ...props }: AppLinkProps) {
  const router = useRouter({ warn: false }) as ReturnType<typeof useRouter> | undefined
  if (!router)
    return (
      <a href={href} {...props}>
        {children}
      </a>
    )
  const linkProps = {
    to: href as never,
    ...props,
    children,
  } as unknown as ComponentProps<typeof Link>
  return <Link {...linkProps} />
}

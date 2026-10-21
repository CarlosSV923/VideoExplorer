import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'

import { AppLink } from './AppLink'

async function renderAppLink() {
  const root = createRootRoute({
    component: () => (
      <>
        <AppLink href="/target">Target</AppLink>
        <Outlet />
      </>
    ),
  })
  const target = createRoute({
    getParentRoute: () => root,
    path: '/target',
    component: () => <h1>Target page</h1>,
  })
  const history = createMemoryHistory({ initialEntries: ['/'] })
  const router = createRouter({ routeTree: root.addChildren([target]), history })
  await router.load()
  render(<RouterProvider router={router} />)
  return router
}

describe('AppLink', () => {
  it('renders a real anchor with href and navigates through TanStack Router', async () => {
    const user = userEvent.setup()
    const router = await renderAppLink()
    const link = screen.getByRole('link', { name: 'Target' })
    expect(link.tagName).toBe('A')
    expect(link).toHaveAttribute('href', '/target')

    await act(async () => {
      await user.click(link)
    })

    expect(router.state.location.pathname).toBe('/target')
    expect(screen.getByRole('heading', { name: 'Target page' })).toBeInTheDocument()
  })

  it('leaves modified clicks to the browser for opening another tab', async () => {
    const router = await renderAppLink()
    const link = screen.getByRole('link', { name: 'Target' })

    fireEvent.click(link, { ctrlKey: true })

    expect(router.state.location.pathname).toBe('/')
    expect(link).toHaveAttribute('href', '/target')
  })
})

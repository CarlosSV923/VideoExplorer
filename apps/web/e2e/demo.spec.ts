import { expect, test as base, type Page } from '@playwright/test'

type DemoFixtures = { externalHosts: string[] }

const test = base.extend<DemoFixtures>({
  externalHosts: async ({ page }, continueTest) => {
    const hosts: string[] = []
    const localOrigin = new URL('http://127.0.0.1:4173').origin
    page.on('request', (request) => {
      const url = new URL(request.url())
      if (url.origin !== localOrigin) hosts.push(url.hostname)
    })
    await page.route('**/*', async (route) => {
      const url = new URL(route.request().url())
      if (url.origin === localOrigin) await route.continue()
      else await route.abort()
    })
    await continueTest(hosts)
  },
})

async function chooseDuplicate(page: Page) {
  const desktopPreview = page.getByRole('button', { name: 'Previsualizar', exact: true })
  const mobile = (page.viewportSize()?.width ?? 1024) < 768
  if (mobile) {
    await page.getByRole('button', { name: 'Así quedará', exact: true }).click()
    const previewSheet = page.getByRole('dialog')
    await expect(previewSheet.getByRole('button', { name: 'Guardar', exact: true })).toBeDisabled()
    await previewSheet.getByRole('button', { name: 'Cerrar' }).click()
  } else {
    await desktopPreview.click()
  }
  await expect(page.getByRole('group', { name: 'Decisión del duplicado' })).toBeVisible()
  await page.getByRole('button', { name: 'Reemplazar', exact: true }).click()
  await desktopPreview.click()
}

async function clickInternalPath(page: Page, path: string) {
  if (path === '/no-asignados' && (page.viewportSize()?.width ?? 1024) < 768) {
    await page
      .getByRole('navigation', { name: 'Navegación móvil' })
      .getByRole('link', { name: 'No asignados' })
      .click()
  } else {
    if (path === '/no-asignados') {
      await page.getByRole('link', { name: 'VideoExplorer' }).click()
    }
    await page.locator(`a[href="${path}"]`).first().click()
  }
  await expect(page).toHaveURL(path)
}

test('internal navigation preserves demo state and active upload progress without a document reload', async ({
  page,
}) => {
  await page.addInitScript(() => {
    ;(window as Window & { navigationMarker?: string }).navigationMarker = 'alive'
    // Keep simulated uploads active long enough to observe progress across route changes.
    const setInterval = window.setInterval.bind(window)
    window.setInterval = ((handler: TimerHandler, timeout?: number, ...args: unknown[]) =>
      setInterval(
        handler,
        timeout === 90 ? 700 : (timeout ?? 0),
        ...args,
      )) as typeof window.setInterval
  })
  await page.goto('/')
  const mobile = (page.viewportSize()?.width ?? 1024) < 768
  const navigationCount = await page.evaluate(
    () => performance.getEntriesByType('navigation').length,
  )

  if (mobile) {
    await page.getByRole('button', { name: /Subidas/ }).click()
    await page
      .getByRole('dialog', { name: 'Subidas' })
      .getByRole('button', { name: 'Probar con archivos de ejemplo' })
      .click()
  } else {
    await page.getByRole('button', { name: 'Probar con archivos de ejemplo' }).click()
  }
  await page
    .getByRole('dialog', { name: '¿Cómo se relacionan estos archivos?' })
    .getByRole('button', { name: 'Sin relación' })
    .click()
  const panel = mobile
    ? page.getByRole('dialog', { name: 'Subidas' })
    : page.getByRole('complementary', { name: 'Subidas' })
  const progress = panel
    .locator('article')
    .filter({ hasText: 'Dark.S02E05.1080p.mkv' })
    .getByRole('progressbar')
  await expect(progress).toBeVisible()
  const initialProgress = Number(await progress.getAttribute('aria-valuenow'))
  await panel.getByRole('button', { name: 'Cerrar' }).click()

  await page
    .getByRole('heading', { name: 'Películas agregadas recientemente' })
    .locator('xpath=../..')
    .locator('a[href^="/peliculas/"]')
    .first()
    .click()
  await expect(page).toHaveURL(/\/peliculas\/\d+$/)
  await page.getByRole('button', { name: /Subidas/ }).click()
  const detailProgress = panel
    .locator('article')
    .filter({ hasText: 'Dark.S02E05.1080p.mkv' })
    .getByRole('progressbar')
  await expect
    .poll(async () => Number(await detailProgress.getAttribute('aria-valuenow')))
    .toBeGreaterThan(initialProgress)
  await panel.getByRole('button', { name: 'Cerrar' }).click()
  const moviesLink = mobile
    ? page
        .getByRole('navigation', { name: 'Navegación móvil' })
        .getByRole('link', { name: 'Películas' })
    : page
        .getByRole('navigation', { name: 'Navegación principal' })
        .getByRole('link', { name: 'Películas' })
  await moviesLink.click()
  await expect(page).toHaveURL('/peliculas')
  await clickInternalPath(page, '/no-asignados')
  await expect(page.getByRole('heading', { name: 'No asignados', exact: true })).toBeVisible()
  expect(
    await page.evaluate(() => (window as Window & { navigationMarker?: string }).navigationMarker),
  ).toBe('alive')
  expect(await page.evaluate(() => performance.getEntriesByType('navigation').length)).toBe(
    navigationCount,
  )
  await expect(page.getByRole('heading', { name: 'Copia de Samba' })).toBeVisible()
  await page.getByRole('button', { name: /Subidas/ }).click()
  const finalProgress = panel
    .locator('article')
    .filter({ hasText: 'Dark.S02E05.1080p.mkv' })
    .getByRole('progressbar')
  await expect
    .poll(async () => Number(await finalProgress.getAttribute('aria-valuenow')))
    .toBeGreaterThan(initialProgress)
  await panel.getByRole('button', { name: 'Cerrar' }).click()
})

test('home, movie and series details show seeded content and seasons', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('status')).toContainText('Modo demo')
  await expect(page.getByRole('heading', { level: 1, name: 'Inicio' })).toBeAttached()
  await expect(
    page.getByRole('heading', { name: 'Películas agregadas recientemente' }),
  ).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Series agregadas recientemente' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Agregados recientemente' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Colecciones' })).toHaveCount(0)

  await page
    .getByRole('heading', { name: 'Películas agregadas recientemente' })
    .locator('xpath=../..')
    .locator('a[href^="/peliculas/"]')
    .first()
    .click()
  await expect(page).toHaveURL(/\/peliculas\/\d+$/)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

  await page.goto('/')
  await page
    .getByRole('heading', { name: 'Series agregadas recientemente' })
    .locator('xpath=../..')
    .locator('a[href^="/series/"]')
    .first()
    .click()
  await expect(page).toHaveURL(/\/series\/\d+$/)
  await expect(page.getByText('Temporadas', { exact: true })).toBeVisible()
})

test('a missing demo movie shows a translated empty state and a return link', async ({ page }) => {
  await page.goto('/peliculas/1')
  await expect(
    page.getByRole('heading', { name: 'Este título ya no está en la biblioteca' }),
  ).toBeVisible()
  await expect(page.getByRole('link', { name: 'Volver a Películas' })).toHaveAttribute(
    'href',
    '/peliculas',
  )
  await expect(page.getByRole('button', { name: 'Reintentar' })).toHaveCount(0)
})

test('search finds a seeded movie', async ({ page }) => {
  await page.goto('/buscar?q=dark')
  await expect(page.getByRole('heading', { name: 'Buscar en la biblioteca' })).toBeVisible()
  await expect(
    page.getByText('Batman: El caballero de la noche', { exact: true }).first(),
  ).toBeVisible()
})

test('demo examples ask for a relation, unlock multipart archives, and reach Unassigned', async ({
  page,
}) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1, name: 'Inicio' })).toBeAttached()
  if ((page.viewportSize()?.width ?? 1024) < 768) {
    await page.getByRole('button', { name: /Subidas/ }).click()
    await page
      .getByRole('dialog', { name: 'Subidas' })
      .getByRole('button', { name: 'Probar con archivos de ejemplo' })
      .click()
  } else await page.getByRole('button', { name: 'Probar con archivos de ejemplo' }).click()
  const relation = page.getByRole('dialog', { name: '¿Cómo se relacionan estos archivos?' })
  await expect(relation).toBeVisible()
  await relation.getByRole('button', { name: 'Capítulos de una misma serie' }).click()

  const firstPartPassword = page
    .locator('article')
    .filter({ hasText: 'Inception.2010.part1.rar' })
    .getByRole('textbox', { name: 'Contraseña' })
  await expect(firstPartPassword).toBeVisible()
  await expect(
    page.getByText('Esta contraseña se aplica a todos los volúmenes del conjunto.').first(),
  ).toBeVisible()
  await firstPartPassword.fill('demo')
  await page
    .locator('article')
    .filter({ hasText: 'Inception.2010.part1.rar' })
    .getByRole('button', { name: 'Continuar' })
    .click()

  await expect(
    page
      .locator('article')
      .filter({ hasText: 'Dark.S02E05.1080p.mkv' })
      .getByText('En No asignados'),
  ).toBeVisible()

  await page.getByLabel('Subidas').getByRole('button', { name: 'Cerrar' }).click()
  await clickInternalPath(page, '/no-asignados')
  await expect(page).toHaveURL('/no-asignados')
  await expect(page.getByRole('heading', { name: 'No asignados', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Volver a leer' }).click()
  await expect(page.locator('article').filter({ hasText: 'Inception.2010' })).toBeVisible()
})

test('assigns the seeded Dark Knight duplicate through save', async ({ page }) => {
  await page.goto('/no-asignados/4/asignar')
  await expect(page.getByRole('heading', { name: /Asignar/ })).toBeVisible()
  await chooseDuplicate(page)

  const save = page.getByRole('button', { name: 'Guardar', exact: true }).last()
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page).toHaveURL('/no-asignados')
})

test('restores a trash item and shows TMDB attribution in settings', async ({ page }) => {
  await page.goto('/ajustes/papelera')
  const entry = page.locator('article').filter({ hasText: 'Inception.2010.part1.rar' })
  await entry.getByRole('button', { name: /Restaurar/ }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: 'Plan de restauración' })).toBeVisible()
  await dialog.getByRole('combobox').selectOption('replace')
  await dialog.getByRole('button', { name: 'Restaurar', exact: true }).click()
  await expect(dialog).toBeHidden()

  await clickInternalPath(page, '/no-asignados')
  await page.getByRole('button', { name: 'Volver a leer' }).click()
  await expect(
    page.locator('article').filter({ hasText: 'Inception.2010.part1.rar' }),
  ).toBeVisible()

  await page.goto('/ajustes/general')
  await expect(
    page.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.'),
  ).toBeVisible()
})

test('reloads a deep route and uses only TMDB images, with placeholders when blocked', async ({
  page,
  externalHosts,
}) => {
  await page.goto('/ajustes/papelera')
  await expect(page.getByRole('heading', { name: 'Papelera', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Papelera', exact: true })).toBeVisible()

  await page.goto('/')
  await expect(
    page.getByRole('heading', { name: 'Películas agregadas recientemente' }),
  ).toBeVisible()
  await expect(page.locator('.bg-surface-missing span[aria-hidden="true"]').first()).toBeVisible()
  expect(externalHosts.length).toBeGreaterThan(0)
  expect(new Set(externalHosts)).toEqual(new Set(['image.tmdb.org']))
})

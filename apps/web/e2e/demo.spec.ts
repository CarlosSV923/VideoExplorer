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
  if (path === '/unassigned' && (page.viewportSize()?.width ?? 1024) < 768) {
    await page
      .getByRole('navigation', { name: 'Navegación móvil' })
      .getByRole('link', { name: 'No asignados' })
      .click()
  } else {
    if (path === '/unassigned') {
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
    .getByRole('dialog', { name: '¿Cómo se relacionan estos 2 archivos?' })
    .getByRole('button', { name: /Sin relación/ })
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
    .locator('a[href^="/movies/"]')
    .first()
    .click()
  await expect(page).toHaveURL(/\/movies\/\d+$/)
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
  await expect(page).toHaveURL('/movies')
  await clickInternalPath(page, '/unassigned')
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
    .locator('a[href^="/movies/"]')
    .first()
    .click()
  await expect(page).toHaveURL(/\/movies\/\d+$/)
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
  await page.goto('/movies/1')
  await expect(
    page.getByRole('heading', { name: 'Este título ya no está en la biblioteca' }),
  ).toBeVisible()
  await expect(page.getByRole('link', { name: 'Volver a Películas' })).toHaveAttribute(
    'href',
    '/movies',
  )
  await expect(page.getByRole('button', { name: 'Reintentar' })).toHaveCount(0)
})

test('search finds a seeded movie', async ({ page }) => {
  await page.goto('/search?q=dark')
  await expect(page.getByRole('heading', { name: 'Buscar en la biblioteca' })).toBeVisible()
  await expect(
    page.getByText('Batman: El caballero de la noche', { exact: true }).first(),
  ).toBeVisible()
})

test('selects two demo movies and restores them in Settings trash on desktop and touch mobile', async ({
  page,
}) => {
  await page.goto('/settings/trash')
  await expect(page.locator('article').first()).toBeVisible()
  const previousEntries = await page.locator('article').count()
  await page.goto('/movies')
  await page.getByRole('button', { name: 'Seleccionar', exact: true }).click()
  const posters = page.getByRole('checkbox')
  await expect(posters.first()).toBeVisible()
  await posters.nth(0).click()
  await posters.nth(1).click()
  await page.getByRole('button', { name: 'Mandar a la papelera', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('2 títulos')
  await dialog.getByRole('button', { name: 'Mandar a la papelera', exact: true }).click()
  await expect(
    page.getByRole('status').filter({ hasText: '2 títulos en la papelera' }),
  ).toBeVisible()
  await page.locator('a[href="/settings/general"]').first().click()
  await page.locator('a[href="/settings/trash"]').first().click()
  await expect(page.locator('article')).toHaveCount(previousEntries + 2)
})

test('selection mode keeps every movie poster frame in the same top-aligned position', async ({
  page,
}) => {
  await page.goto('/movies')
  await expect(page.locator('a[href^="/movies/"]').first()).toBeVisible()
  const grid = page.locator('main div.grid').first()
  const setMixedLengthTitles = () =>
    grid.evaluate((element) => {
      const titles = Array.from(element.children)
        .slice(0, 2)
        .map((poster) => poster.children[1])
      if (!titles[0] || !titles[1]) throw new Error('Expected two movie posters')
      titles[0].textContent = 'Short'
      titles[1].textContent =
        'A deliberately long title that wraps into three lines on this poster grid'
      return titles.map((title) => {
        const lineHeight = Number.parseFloat(window.getComputedStyle(title).lineHeight)
        return Math.round(title.getBoundingClientRect().height / lineHeight)
      })
    })
  const readFrames = () =>
    grid.evaluate((element) =>
      Array.from(element.children).map((poster) => {
        const frame = poster.firstElementChild
        const title = frame?.nextElementSibling
        return {
          top: frame ? frame.getBoundingClientRect().top + window.scrollY : undefined,
          left: frame?.getBoundingClientRect().left,
          width: frame?.getBoundingClientRect().width,
          height: frame?.getBoundingClientRect().height,
          lines: title
            ? Math.round(
                title.getBoundingClientRect().height /
                  Number.parseFloat(window.getComputedStyle(title).lineHeight),
              )
            : 0,
        }
      }),
    )

  expect(await setMixedLengthTitles()).toEqual([1, 3])
  const before = await readFrames()
  expect(before.length).toBeGreaterThan(1)
  const rows = new Map<number, number[]>()
  for (const frame of before) {
    const top = Math.round(frame.top ?? -1)
    rows.set(top, [...(rows.get(top) ?? []), frame.lines])
  }
  expect([...rows.values()][0]).toContain(1)
  expect([...rows.values()][0]).toContain(3)

  await page.getByRole('button', { name: 'Seleccionar', exact: true }).click()
  await expect(page.getByRole('checkbox').first()).toBeVisible()
  expect(await setMixedLengthTitles()).toEqual([1, 3])
  const after = await readFrames()
  expect(after).toHaveLength(before.length)
  for (const [index, frame] of after.entries()) {
    expect(frame.top).toBe(before[index]?.top)
    expect(frame.left).toBe(before[index]?.left)
    expect(frame.width).toBe(before[index]?.width)
    expect(frame.height).toBe(before[index]?.height)
  }

  await page.getByRole('button', { name: 'Cancelar', exact: true }).first().click()
  await expect(page.getByRole('button', { name: 'Seleccionar', exact: true })).toBeVisible()
  expect(await setMixedLengthTitles()).toEqual([1, 3])
  const afterCancel = await readFrames()
  for (const [index, frame] of afterCancel.entries()) {
    expect(frame.top).toBe(before[index]?.top)
    expect(frame.left).toBe(before[index]?.left)
    expect(frame.width).toBe(before[index]?.width)
    expect(frame.height).toBe(before[index]?.height)
  }
})

test('filters the demo grid by two genres on desktop and touch mobile', async ({ page }) => {
  await page.goto('/movies')
  await expect(page.getByRole('link', { name: /El rey le.n/ }).first()).toBeVisible()
  const menu = page.getByRole('button', { name: 'Géneros', exact: true })
  await menu.click()
  await page.getByRole('checkbox', { name: 'Animación' }).check()
  await page.getByRole('checkbox', { name: 'Drama' }).check()
  await expect(page.getByRole('button', { name: 'Géneros · 2' })).toBeVisible()
  await expect(page).toHaveURL(/genre=Animaci%C3%B3n%2CDrama/)
  await expect(page.getByRole('link', { name: /El rey le.n/ }).first()).toBeVisible()
  await expect(page.getByRole('link', { name: /Duna/ })).toHaveCount(0)
  if ((page.viewportSize()?.width ?? 1024) < 768) {
    const panelId = await page
      .getByRole('button', { name: 'Géneros · 2' })
      .getAttribute('aria-controls')
    if (!panelId) throw new Error('The genre button must control its open panel')
    const panel = page.locator(`#${panelId}`)
    const box = await panel.boundingBox()
    expect(box?.width).toBeCloseTo((page.viewportSize()?.width ?? 390) - 32, 0)
  }
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
  const relation = page.getByRole('dialog', { name: '¿Cómo se relacionan estos 2 archivos?' })
  await expect(relation).toBeVisible()
  await expect(
    relation.getByText('Un título dividido en varios archivos (CD1, CD2…)'),
  ).toBeVisible()
  await expect(relation.getByText('Cada archivo es uno o varios capítulos')).toBeVisible()
  await expect(relation.getByText('Cada archivo o comprimido es independiente')).toBeVisible()
  const cards = relation
    .getByRole('button')
    .filter({ hasText: /Partes de una misma|Capítulos de una misma|Sin relación/ })
  await expect(cards).toHaveCount(3)
  const cardBoxes = await cards.evaluateAll((buttons) =>
    buttons.map((button) => {
      const { x, y, width, height } = button.getBoundingClientRect()
      return { x, y, width, height }
    }),
  )
  expect(cardBoxes[0]?.width).toBeGreaterThan(0)
  expect(cardBoxes[1]?.y).toBeGreaterThan(cardBoxes[0]?.y ?? 0)
  expect(cardBoxes[2]?.y).toBeGreaterThan(cardBoxes[1]?.y ?? 0)
  const partsCard = relation.getByRole('button', { name: /Partes de una misma película o serie/ })
  const episodesCard = relation.getByRole('button', { name: /Capítulos de una misma serie/ })
  expect(await partsCard.evaluate((element) => element === document.activeElement)).toBe(true)
  await partsCard.press('ArrowDown')
  expect(await episodesCard.evaluate((element) => element === document.activeElement)).toBe(true)
  await episodesCard.press('ArrowDown')
  await relation.getByRole('button', { name: 'Sin relación' }).focus()
  await relation.getByRole('button', { name: 'Sin relación' }).press('ArrowUp')
  expect(await episodesCard.evaluate((element) => element === document.activeElement)).toBe(true)
  await episodesCard.click()

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
  await clickInternalPath(page, '/unassigned')
  await expect(page).toHaveURL('/unassigned')
  await expect(page.getByRole('heading', { name: 'No asignados', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Volver a leer' }).click()
  await expect(page.locator('article').filter({ hasText: 'Inception.2010' })).toBeVisible()
})

test('shows the stacked relation choices in English', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('ve.lang', 'en')
  })
  await page.goto('/')
  if ((page.viewportSize()?.width ?? 1024) < 768) {
    await page.getByRole('button', { name: /Uploads/ }).click()
    await page
      .getByRole('dialog', { name: 'Uploads' })
      .getByRole('button', { name: 'Try sample files' })
      .click()
  } else await page.getByRole('button', { name: 'Try sample files' }).click()

  const relation = page.getByRole('dialog', { name: 'How are these 2 files related?' })
  await expect(relation).toBeVisible()
  await expect(relation.getByText('One title split across several files (CD1, CD2…)')).toBeVisible()
  await expect(relation.getByText('Each file contains one or more episodes')).toBeVisible()
  await expect(relation.getByText('Each file or archive is independent')).toBeVisible()
  expect(
    await relation
      .getByRole('button', { name: /Parts of one movie or series/ })
      .evaluate((element) => element === document.activeElement),
  ).toBe(true)
})

test('keeps mobile hero buttons and indicators in separate rows in Spanish and English', async ({
  page,
}) => {
  for (const width of [390, 360]) {
    await page.setViewportSize({ width, height: 844 })
    for (const language of ['es', 'en']) {
      await page.goto('/')
      await page.evaluate((selected) => {
        localStorage.setItem('ve.lang', selected)
      }, language)
      await page.reload()
      const indicators = page.getByRole('group', {
        name: language === 'en' ? 'Featured indicators' : 'Indicadores de destacados',
      })
      await expect(indicators).toBeVisible()
      const buttons = indicators.locator('xpath=preceding-sibling::div[1]')
      const buttonBox = await buttons.boundingBox()
      const indicatorBox = await indicators.boundingBox()
      expect(buttonBox).not.toBeNull()
      expect(indicatorBox).not.toBeNull()
      expect((indicatorBox?.y ?? 0) - ((buttonBox?.y ?? 0) + (buttonBox?.height ?? 0))).toBe(12)
    }
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')
  const hero = page.locator('main section[aria-label="Featured"]')
  await expect(hero).toBeVisible()
  expect((await hero.boundingBox())?.height).toBe(600)
  expect(
    await hero
      .locator('p.text-body-lg')
      .evaluate((element) => element.getBoundingClientRect().height),
  ).toBe(81)
})

test('updates demo catalog text after changing language without reloading', async ({ page }) => {
  await page.goto('/settings/general')
  await page.getByRole('button', { name: 'English' }).click()
  await page.goto('/movies')
  await expect(page.getByRole('link', { name: /The Lion King/ }).first()).toBeVisible()
  await page.goto('/settings/general')
  await page.getByRole('button', { name: 'Español' }).click()
  await page.goto('/movies')
  await expect(page.getByRole('link', { name: /El rey le.n/ }).first()).toBeVisible()
})

test('assigns the seeded Dark Knight duplicate through save', async ({ page }) => {
  await page.goto('/unassigned/4/assign')
  await expect(page.getByRole('heading', { name: /Asignar/ })).toBeVisible()
  await chooseDuplicate(page)

  const save = page.getByRole('button', { name: 'Guardar', exact: true }).last()
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page).toHaveURL('/unassigned')
})

test('shows selectable title matches with posters in Edit and Assign on desktop and mobile', async ({
  page,
}) => {
  await page.goto('/movies/1000/edit')
  await page.getByRole('textbox', { name: 'Coincidencia de TMDB' }).click()
  const editDialog = page.getByRole('dialog')
  await editDialog.getByRole('textbox', { name: 'Buscar título' }).fill('dark')
  await expect(editDialog.getByRole('radio').first()).toBeVisible()
  await expect(editDialog.getByRole('radiogroup').first()).toHaveAttribute('aria-labelledby', /.+/)
  const editOption = editDialog.getByRole('radio').first()
  await expect(editOption).toHaveAttribute('aria-checked', 'false')
  await expect(editOption.locator('img')).toHaveAttribute('loading', 'lazy')
  await editOption.scrollIntoViewIfNeeded()
  await expect
    .poll(async () => {
      const image = editOption.locator('img')
      if ((await image.count()) === 0) return true
      return image.evaluate((element) => (element as HTMLImageElement).complete)
    })
    .toBe(true)
  await page.screenshot({
    path: `test-results/title-picker-edit-${String(page.viewportSize()?.width ?? 0)}.png`,
    animations: 'disabled',
  })

  await page.goto('/unassigned/4/assign')
  await page
    .getByRole('button', { name: /Cambiar/ })
    .first()
    .click()
  const assignDialog = page.getByRole('dialog')
  await assignDialog.getByRole('textbox', { name: 'Buscar título' }).fill('dark')
  const assignOption = assignDialog.getByRole('radio').first()
  await expect(assignOption).toBeVisible()
  await expect(assignOption).toHaveAttribute('aria-checked', 'true')
  await expect(assignOption.locator('img')).toHaveAttribute('loading', 'lazy')
  await assignOption.scrollIntoViewIfNeeded()
  await expect
    .poll(async () => {
      const image = assignOption.locator('img')
      if ((await image.count()) === 0) return true
      return image.evaluate((element) => (element as HTMLImageElement).complete)
    })
    .toBe(true)
  await page.screenshot({
    path: `test-results/title-picker-assign-${String(page.viewportSize()?.width ?? 0)}.png`,
    animations: 'disabled',
  })
})

test('restores a trash item and shows TMDB attribution in settings', async ({ page }) => {
  await page.goto('/settings/trash')
  const entry = page.locator('article').filter({ hasText: 'Inception.2010.part1.rar' })
  await entry.getByRole('button', { name: /Restaurar/ }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: 'Plan de restauración' })).toBeVisible()
  await dialog.getByRole('combobox').selectOption('replace')
  await dialog.getByRole('button', { name: 'Restaurar', exact: true }).click()
  await expect(dialog).toBeHidden()

  await clickInternalPath(page, '/unassigned')
  await page.getByRole('button', { name: 'Volver a leer' }).click()
  await expect(
    page.locator('article').filter({ hasText: 'Inception.2010.part1.rar' }),
  ).toBeVisible()

  await page.goto('/settings/general')
  await expect(
    page.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.'),
  ).toBeVisible()
})

test('reloads a deep route and uses only TMDB images, with placeholders when blocked', async ({
  page,
  externalHosts,
}) => {
  await page.goto('/settings/trash')
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

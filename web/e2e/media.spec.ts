import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from '@playwright/test'
import { expectAccessible, login, unique } from './helpers'

const photoBytes = readFileSync(fileURLToPath(new URL('./fixtures/family-photo.jpg', import.meta.url)))

/**
 * The fixture with a unique tail after the JPEG end marker: decoders ignore
 * it, but it changes the hash, so tests sharing the database do not get each
 * other's deduplicated upload.
 */
const uniquePhoto = (tag: string) => ({
  name: 'family-photo.jpg',
  mimeType: 'image/jpeg',
  buffer: Buffer.concat([photoBytes, Buffer.from(tag)]),
})

test('upload a photo, match the imported face and use it as portrait', async ({ page }) => {
  await login(page)
  const surname = unique('Media', page)
  const rose = (await (await page.request.post('/api/persons', { data: { givenNames: 'Rose', surname } })).json()) as { id: number }
  const paul = (await (await page.request.post('/api/persons', { data: { givenNames: 'Paul', surname } })).json()) as { id: number }

  await page.goto(`/people/${paul.id}`)
  const gallery = page.getByRole('region', { name: 'Photos and documents' })
  await gallery.getByLabel('+ Upload files').setInputFiles(uniquePhoto(surname))
  const tile = gallery.getByRole('link', { name: 'family-photo' })
  await expect(tile).toBeVisible()
  await expectAccessible(page)
  await tile.click()

  // The face tag from the file's XMP is there, waiting to be matched.
  await expect(page.getByRole('heading', { level: 1, name: 'family-photo' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Face: Grandma Rose' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Who is “Grandma Rose”?' }).fill(surname)
  await page.getByRole('button', { name: new RegExp(`^Rose ${surname}`) }).click()
  await expect(page.getByRole('button', { name: `Face: Rose ${surname}` })).toBeVisible()
  await expectAccessible(page)

  await page.getByRole('button', { name: new RegExp(`^Use as portrait\\s?: Rose ${surname}$`) }).click()
  await page.goto(`/people/${rose.id}`)
  // The portrait is the cropped face, and the photo is in her gallery.
  await expect(page.locator('img[src*="region="]').first()).toBeVisible()
  await expect(page.getByRole('region', { name: 'Photos and documents' }).getByRole('link', { name: 'family-photo' })).toBeVisible()
})

test('draw a face box', async ({ page }) => {
  await login(page)
  const surname = unique('Draw', page)
  await page.request.post('/api/persons', { data: { givenNames: 'Otto', surname } })
  const upload = await page.request.post('/api/media', {
    multipart: { file: { ...uniquePhoto(surname), name: 'draw.jpg' } },
  })
  const media = (await upload.json()) as { id: number }
  await page.goto(`/media/${media.id}`)

  const img = page.getByRole('img').first()
  await expect(img).toBeVisible()
  await page.getByRole('button', { name: 'Mark a face' }).click()
  const box = (await img.boundingBox())!
  await page.mouse.move(box.x + box.width * 0.55, box.y + box.height * 0.3)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width * 0.75, box.y + box.height * 0.6, { steps: 5 })
  await page.mouse.up()

  await expect(page.getByText('Who is this?')).toBeVisible()
  await page.getByRole('searchbox', { name: 'Person', exact: true }).fill(surname)
  await page.getByRole('button', { name: new RegExp(`^Otto ${surname}`) }).click()
  await page.getByRole('button', { name: 'Save face' }).click()
  await expect(page.getByRole('button', { name: `Face: Otto ${surname}` })).toBeVisible()
})

test('unsupported files are refused with a message', async ({ page }) => {
  await login(page)
  const p = (await (await page.request.post('/api/persons', { data: { givenNames: unique('Svg', page) } })).json()) as { id: number }
  await page.goto(`/people/${p.id}`)
  await page.getByLabel('+ Upload files').setInputFiles({
    name: 'evil.svg',
    mimeType: 'image/svg+xml',
    buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>'),
  })
  await expect(page.getByRole('alert')).toContainText('evil.svg: This kind of file is not supported')
})

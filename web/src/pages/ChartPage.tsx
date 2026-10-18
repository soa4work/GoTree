import { useQuery } from '@tanstack/react-query'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { getPerson, getTree } from '../api/endpoints'
import type { PersonRef } from '../api/types'
import { bounds, photoUrl } from '../chart/geometry'
import { WallChart } from '../chart/WallChart'
import { chartThemes, type ChartThemeName } from '../chart/themes'
import { fitChart, MIN_READABLE_PT, type Orientation, type PaperName, papers } from '../chart/paper'
import { Button } from '../components/Button'
import { SelectField } from '../components/Field'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { formatDecimal } from '../lib/collate'
import { formatDay } from '../lib/dates'
import { mapLimit } from '../lib/mapLimit'
import { fullName } from '../lib/people'
import { PersonPicker } from '../people/PersonPicker'
import { layoutTree, type TreeLayout, type TreeView } from '../tree/layout'
import { indexGraph, type TreeIndex } from '../tree/model'

const views: TreeView[] = ['pedigree', 'descendants', 'hourglass']

async function asDataUri(url: string): Promise<string> {
  const res = await fetch(url, { credentials: 'same-origin' })
  if (!res.ok) throw new Error(`photo ${res.status}`)
  const blob = await res.blob()
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
  })
}

export function ChartPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const rootId = Number(params.get('root')) || null
  const [view, setView] = useState<TreeView>('pedigree')
  const [generations, setGenerations] = useState(4)
  const [bloodOnly, setBloodOnly] = useState(false)
  const [photos, setPhotos] = useState(true)
  const [paper, setPaper] = useState<PaperName>('A3')
  const [custom, setCustom] = useState({ w: 600, h: 400 })
  const [orientation, setOrientation] = useState<Orientation>('auto')
  const [title, setTitle] = useState<string | null>(null)
  const [subtitle, setSubtitle] = useState<string | null>(null)
  const [theme, setTheme] = useState<ChartThemeName>('tree')
  const [downloading, setDownloading] = useState(false)
  const [embedded, setEmbedded] = useState<Record<string, string> | null>(null)
  const svgRef = useRef<SVGSVGElement>(null)

  const root = useQuery({
    queryKey: ['person', rootId],
    queryFn: ({ signal }) => getPerson(rootId ?? 0, signal),
    enabled: rootId !== null,
  })
  const up = view === 'descendants' ? 0 : generations
  const down = view === 'pedigree' ? 0 : generations
  const tree = useQuery({
    queryKey: ['tree', rootId, up, down, false],
    queryFn: ({ signal }) => getTree(rootId ?? 0, { up, down, siblings: false }, signal),
    enabled: rootId !== null,
  })
  const index = useMemo(() => (tree.data ? indexGraph(tree.data) : null), [tree.data])
  const layout = useMemo(
    () => (index ? layoutTree(index, { view, generations, bloodOnly }) : null),
    [index, view, generations, bloodOnly],
  )
  const rootName = root.data ? (fullName(root.data) ?? t('person.unknown')) : ''
  const chartTitle =
    title ?? (rootName ? t(`chart.defaultTitle.${view}` as 'chart.defaultTitle.pedigree', { name: rootName }) : '')
  const chartSubtitle =
    subtitle ??
    (index
      ? t('chart.defaultSubtitle', {
          count: index.persons.size,
          date: formatDay(new Date().toISOString()),
        })
      : '')
  const size = paper === 'custom' ? custom : papers[paper]
  const fit = layout ? fitChart(bounds(layout).w, bounds(layout).h, size, orientation, chartTitle !== '') : null
  const rootRef: PersonRef | null = root.data ? { ...root.data, birthDate: '', deathDate: '' } : null

  const download = async () => {
    if (!index || !layout || !svgRef.current || downloading) return
    setDownloading(true)
    try {
      await save(index, layout)
    } finally {
      setDownloading(false)
    }
  }

  const save = async (index: TreeIndex, layout: TreeLayout) => {
    // A file on its own cannot load photos from the server, so they go in.
    // Only the people drawn on the chart need one, and a few at a time.
    const urls = photos
      ? [
          ...new Set(
            layout.nodes
              .filter((n) => n.kind === 'person')
              .map((n) => photoUrl(index.persons.get(n.personId!)!))
              .filter((u): u is string => u !== null),
          ),
        ]
      : []
    const pairs = await mapLimit(urls, 4, async (u) => [u, await asDataUri(u).catch(() => u)] as const)
    const map: Record<string, string> = Object.fromEntries(pairs)
    setEmbedded(map)
    // Let React render the embedded images before serialising.
    await new Promise((r) => setTimeout(r, 0))
    const svg = svgRef.current?.outerHTML ?? ''
    setEmbedded(null)
    const url = URL.createObjectURL(
      new Blob([`<?xml version="1.0" encoding="UTF-8"?>\n${svg}`], { type: 'image/svg+xml' }),
    )
    const a = document.createElement('a')
    a.href = url
    a.download = `${chartTitle || t('chart.fileName')}.svg`.replace(/[\\/:*?"<>|]/g, '-')
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-6">
      {fit && <style>{`@page { size: ${fit.paperW}mm ${fit.paperH}mm; margin: 0 }`}</style>}
      <div className="max-w-4xl space-y-4 print:hidden">
        <PageHeading title={t('chart.title')}>{t('chart.title')}</PageHeading>
        <p className="text-slate-700 dark:text-slate-300">{t('chart.intro')}</p>
        <PersonPicker
          label={t('chart.root')}
          value={rootRef}
          onChange={(p) => {
            setParams(p ? { root: String(p.id) } : {}, { replace: true })
            setTitle(null)
            setSubtitle(null)
          }}
        />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <SelectField label={t('tree.view')} value={view} onChange={(e) => setView(e.target.value as TreeView)}>
            {views.map((v) => (
              <option key={v} value={v}>
                {t(`tree.views.${v}`)}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t('tree.generations')}
            value={generations}
            onChange={(e) => setGenerations(Number(e.target.value))}
          >
            {Array.from({ length: 10 }, (_, i) => i + 1).map((g) => (
              <option key={g} value={g}>
                {g}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('chart.paper')} value={paper} onChange={(e) => setPaper(e.target.value as PaperName)}>
            {Object.keys(papers).map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
            <option value="custom">{t('chart.custom')}</option>
          </SelectField>
          <SelectField
            label={t('chart.orientation')}
            value={orientation}
            onChange={(e) => setOrientation(e.target.value as Orientation)}
          >
            {(['auto', 'portrait', 'landscape'] as const).map((o) => (
              <option key={o} value={o}>
                {t(`chart.orientation_${o}`)}
              </option>
            ))}
          </SelectField>
        </div>
        {paper === 'custom' && (
          <div className="grid max-w-md gap-4 sm:grid-cols-2">
            <TextField
              type="number"
              min={100}
              max={5000}
              label={t('chart.widthMm')}
              value={custom.w}
              onChange={(e) => setCustom({ ...custom, w: Math.max(100, Number(e.target.value) || 100) })}
            />
            <TextField
              type="number"
              min={100}
              max={5000}
              label={t('chart.heightMm')}
              value={custom.h}
              onChange={(e) => setCustom({ ...custom, h: Math.max(100, Number(e.target.value) || 100) })}
            />
          </div>
        )}
        <div className="grid gap-4 sm:grid-cols-[1fr_1fr_12rem]">
          <TextField label={t('chart.chartTitle')} value={chartTitle} onChange={(e) => setTitle(e.target.value)} />
          <TextField
            label={t('chart.chartSubtitle')}
            value={chartSubtitle}
            disabled={chartTitle === ''}
            onChange={(e) => setSubtitle(e.target.value)}
          />
          <SelectField
            label={t('chart.theme')}
            value={theme}
            onChange={(e) => setTheme(e.target.value as ChartThemeName)}
          >
            {Object.keys(chartThemes).map((name) => (
              <option key={name} value={name}>
                {t(`chart.themes.${name}` as 'chart.themes.classic')}
              </option>
            ))}
          </SelectField>
        </div>
        <div className="flex flex-wrap gap-x-6">
          <label className="flex min-h-11 items-center gap-2">
            <input type="checkbox" className="size-5" checked={photos} onChange={(e) => setPhotos(e.target.checked)} />
            {t('chart.photos')}
          </label>
          <label className="flex min-h-11 items-center gap-2">
            <input
              type="checkbox"
              className="size-5"
              checked={bloodOnly}
              disabled={view === 'pedigree'}
              onChange={(e) => setBloodOnly(e.target.checked)}
            />
            {t('tree.bloodOnly')}
          </label>
        </div>
        <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
          {rootId === null
            ? t('chart.pickRoot')
            : tree.isPending
              ? t('app.loading')
              : tree.isError
                ? t('common.loadFailed')
                : fit
                  ? [
                      t('chart.summary', {
                        count: index?.persons.size ?? 0,
                        paper: paper === 'custom' ? `${size.w} × ${size.h} mm` : paper,
                      }),
                      t(fit.landscape ? 'chart.orientation_landscape' : 'chart.orientation_portrait'),
                      t('chart.textSize', { pt: formatDecimal(fit.namePt, 1) }),
                      tree.data?.truncated ? t('tree.truncated') : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')
                  : ''}
        </p>
        {fit && fit.namePt < MIN_READABLE_PT && (
          <p role="alert" className="font-medium text-amber-900 dark:text-amber-200">
            {t('chart.tooSmall', { pt: formatDecimal(fit.namePt, 1) })}
          </p>
        )}
        {fit && (
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => window.print()}>{t('chart.print')}</Button>
            <Button variant="secondary" busy={downloading} onClick={() => void download()}>
              {t('chart.downloadSvg')}
            </Button>
          </div>
        )}
      </div>
      {layout && index && fit && (
        <div className="overflow-hidden rounded-lg border border-slate-300 shadow-sm print:rounded-none print:border-0 print:shadow-none dark:border-slate-700">
          <WallChart
            ref={svgRef}
            layout={layout}
            index={index}
            fit={fit}
            title={chartTitle}
            subtitle={chartSubtitle}
            theme={theme}
            photos={photos}
            photoSrc={embedded ?? undefined}
          />
        </div>
      )}
    </div>
  )
}

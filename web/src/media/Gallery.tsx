import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { listMedia, thumbUrl, type MediaOwner } from '../api/endpoints'
import type { MediaRef } from '../api/types'
import { Button } from '../components/Button'

const kindIcon: Record<string, string> = { document: '📄', audio: '🎙', video: '🎞' }

const pageSize = 120

export function Gallery({ owner }: { owner: MediaOwner | null }) {
  return owner ? <OwnerMedia owner={owner} /> : <AllMedia />
}

function OwnerMedia({ owner }: { owner: MediaOwner }) {
  const media = useQuery({
    queryKey: ['media', owner.entityType, owner.entityId],
    queryFn: ({ signal }) => listMedia(owner, signal),
  })
  return <MediaGrid items={media.data} pending={media.isPending} failed={media.isError} />
}

function AllMedia() {
  const { t } = useTranslation()
  const media = useInfiniteQuery({
    queryKey: ['media', 'all', 0],
    queryFn: ({ pageParam, signal }) => listMedia(null, signal, { limit: pageSize, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => (last.length < pageSize ? undefined : pages.length * pageSize),
  })
  return (
    <>
      <MediaGrid items={media.data?.pages.flat()} pending={media.isPending} failed={media.isError} />
      {media.hasNextPage && (
        <Button variant="secondary" className="mt-4" onClick={() => void media.fetchNextPage()} busy={media.isFetchingNextPage}>
          {t('media.more')}
        </Button>
      )}
    </>
  )
}

function MediaGrid({ items, pending, failed }: { items: MediaRef[] | undefined; pending: boolean; failed: boolean }) {
  const { t } = useTranslation()
  if (pending) return <p role="status">{t('app.loading')}</p>
  if (failed || !items) return <p role="alert">{t('common.loadFailed')}</p>
  if (items.length === 0) return <p className="text-slate-600 dark:text-slate-400">{t('media.none')}</p>
  return (
    <ul className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-6">
      {items.map((m) => (
        <li key={m.id}>
          <MediaTile media={m} />
        </li>
      ))}
    </ul>
  )
}

export function MediaTile({ media }: { media: MediaRef }) {
  const { t } = useTranslation()
  const label = media.title || t(`media.kinds.${media.kind}`)
  return (
    <Link
      to={`/media/${media.id}`}
      className="group block overflow-hidden rounded-lg border border-slate-200 bg-slate-50 dark:border-slate-800 dark:bg-slate-900"
    >
      <span className="block aspect-square">
        {media.kind === 'image' ? (
          <img src={thumbUrl(media.id, 256)} alt="" loading="lazy" className="size-full object-cover group-hover:opacity-90" />
        ) : (
          <span aria-hidden="true" className="flex size-full items-center justify-center text-4xl">
            {kindIcon[media.kind]}
          </span>
        )}
      </span>
      <span className="block truncate px-2 py-1 text-xs">
        {label}
        {media.kind !== 'image' && <span className="sr-only"> ({t(`media.kinds.${media.kind}`)})</span>}
      </span>
    </Link>
  )
}

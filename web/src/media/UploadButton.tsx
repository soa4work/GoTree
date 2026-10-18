import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { uploadMedia, type MediaOwner } from '../api/endpoints'
import type { Media } from '../api/types'
import { errorText } from '../lib/errors'

export const acceptedTypes =
  'image/jpeg,image/png,image/gif,image/webp,application/pdf,audio/mpeg,audio/mp4,audio/x-m4a,audio/ogg,audio/wav,audio/webm,video/mp4,video/webm,.m4a,.mp3,.ogg,.wav,.weba'

interface Props {
  owner: MediaOwner | null
  onUploaded?: (m: Media) => void
}

/** A file picker styled as a button; several files upload one after another. */
export function UploadButton({ owner, onUploaded }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const queryClient = useQueryClient()
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [errors, setErrors] = useState<string[]>([])

  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      const failed: string[] = []
      setErrors([])
      for (const [i, file] of files.entries()) {
        setProgress({ done: i, total: files.length })
        try {
          // Awaited on its own line: in `onUploaded?.(await …)` a missing
          // callback would short-circuit the upload itself.
          const media = await uploadMedia(file, owner)
          onUploaded?.(media)
        } catch (e) {
          failed.push(t('media.uploadFailed', { name: file.name, message: errorText(e, t) }))
        }
      }
      setProgress(null)
      setErrors(failed)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
    },
  })

  return (
    <div>
      <input
        id={id}
        type="file"
        multiple
        accept={acceptedTypes}
        className="peer sr-only"
        disabled={upload.isPending}
        onChange={(e) => {
          const files = [...(e.target.files ?? [])]
          e.target.value = ''
          if (files.length) upload.mutate(files)
        }}
      />
      <label
        htmlFor={id}
        className="inline-flex min-h-11 cursor-pointer items-center rounded-lg border border-slate-400 px-4 font-medium hover:bg-slate-100 peer-focus-visible:outline peer-focus-visible:outline-3 peer-focus-visible:outline-brand-500 peer-disabled:opacity-60 dark:border-slate-600 dark:hover:bg-slate-800"
      >
        + {t('media.upload')}
      </label>
      <p role="status" className="mt-1 text-sm text-slate-600 dark:text-slate-400">
        {progress ? t('media.uploading', { done: progress.done + 1, total: progress.total }) : ''}
      </p>
      {errors.length > 0 && (
        <ul role="alert" className="mt-1 text-sm font-medium text-red-700 dark:text-red-400">
          {errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

/* eslint-disable jsx-a11y/media-has-caption -- recordings are oral history without timed captions; the transcript shown next to the player is their text alternative */

interface Props {
  src: string
  label: string
}

export function AudioPlayer({ src, label }: Props) {
  return (
    // No captions: the transcript next to the player is the text alternative.
    <audio controls preload="metadata" src={src} className="w-full" aria-label={label} />
  )
}

export function VideoPlayer({ src, label }: Props) {
  return (
    // No captions: the transcript next to the player is the text alternative.
    <video controls preload="metadata" src={src} className="max-h-[70dvh] w-full" aria-label={label} />
  )
}

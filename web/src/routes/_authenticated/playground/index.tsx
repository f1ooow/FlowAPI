import { createFileRoute } from '@tanstack/react-router'

import { ImagePlayground } from '@/features/image-playground'

export const Route = createFileRoute('/_authenticated/playground/')({
  component: ImagePlayground,
})

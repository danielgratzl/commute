import type { Direction, LiveResponse } from './types'

export interface Override {
  dir?: Direction
  now?: boolean
}

export class ApiError extends Error {}

export async function fetchLive(o: Override, signal?: AbortSignal): Promise<LiveResponse> {
  const q = new URLSearchParams()
  if (o.dir) q.set('dir', o.dir)
  if (o.now) q.set('when', 'now')
  const res = await fetch(`/api/live${q.size ? '?' + q : ''}`, { signal, cache: 'no-store' })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `Server answered ${res.status}`)
  return body as LiveResponse
}

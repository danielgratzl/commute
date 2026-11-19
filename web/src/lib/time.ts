const TZ = 'Europe/Zurich'

const hm = new Intl.DateTimeFormat('de-CH', { hour: '2-digit', minute: '2-digit', timeZone: TZ })
const day = new Intl.DateTimeFormat('en-GB', { weekday: 'short', day: 'numeric', month: 'short', timeZone: TZ })

export const ms = (iso: string) => new Date(iso).getTime()
export const clock = (t: string | number | Date) => hm.format(new Date(t))
export const dayLabel = (t: string | number | Date) => day.format(new Date(t))

/** "7 min", "1 h 05", "12 h" */
export function inWords(minutes: number): string {
  if (minutes < 60) return `${minutes} min`
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  if (h >= 10 || m === 0) return `${h} h`
  return `${h} h ${String(m).padStart(2, '0')}`
}

/** "4:07" for a countdown in seconds */
export function mmss(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

export function ago(seconds: number): string {
  if (seconds < 10) return 'just now'
  if (seconds < 60) return `${Math.floor(seconds)} s ago`
  return `${Math.floor(seconds / 60)} min ago`
}

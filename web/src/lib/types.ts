export type Mode = 'walk' | 'bus' | 'tram' | 'train' | 'ship' | 'cable' | 'other'
export type Status = 'ok' | 'at_risk' | 'cancelled'
export type Direction = 'h2w' | 'w2h'

export interface Leg {
  mode: Mode
  line?: string
  direction?: string
  from: string
  to: string
  from_stop_id?: string
  to_stop_id?: string
  depart: string
  arrive: string
  dep_delay?: number
  arr_delay?: number
  cancelled?: boolean
  track?: string
  track_changed?: boolean
  minutes: number
}

export interface Connection {
  id: string
  leave_at: string
  door_depart: string
  door_arrive: string
  planned_arrive: string
  minutes: number
  delay_min: number
  transfers: number
  status: Status
  status_note?: string
  realtime_alternative?: boolean
  has_realtime: boolean
  disruptions?: string[]
  legs: Leg[]
  weather: null
}

export interface Plan {
  direction: Direction
  mode: 'live' | 'planned'
  at?: string
  arrive_by?: string
  label: string
  reason: string
  auto: boolean
  commute_day: boolean
}

export interface LiveResponse {
  now: string
  plan: Plan
  from: string
  to: string
  hero_id: string
  connections: Connection[]
  source: string
  fetched_at: string
  stale: boolean
  warning?: string
  budget: { used: number; limit: number }
}

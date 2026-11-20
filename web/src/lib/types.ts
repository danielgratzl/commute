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
  weather: TripWeather | null
}

export interface PointWeather {
  place: 'origin' | 'destination'
  at: string
  temp: number
  feels: number
  rate_mm_h: number
  chance: number
  snow?: boolean
}

export interface OutdoorInterval {
  kind: 'walk' | 'wait' | 'ride'
  leg: number
  start: string
  end: string
  place: 'origin' | 'destination' | 'midway'
  chance: number
  rate_mm_h: number
}

export interface TripWeather {
  origin: PointWeather
  destination: PointWeather
  outdoor_min: number
  chance: number
  rate_mm_h: number
  intensity: 'none' | 'light' | 'moderate' | 'heavy'
  verdict: 'dry' | 'maybe' | 'wet'
  headline: string
  detail: string
  snow?: boolean
  ice?: boolean
  intervals: OutdoorInterval[]
  source: string
  gust_kmh?: number
  dark?: boolean
}

export interface BikeOption {
  leave_at: string
  door_arrive: string
  verdict: 'ride' | 'gear' | 'train' | 'unsafe' | 'unknown'
  headline: string
  weather: TripWeather | null
}

export interface BikeView {
  label: string
  distance_km: number
  ascent_m: number
  descent_m: number
  signals: number
  ride_min: number
  door_min: number
  profile: [number, number][]
  best: BikeOption
  best_index: number
  options: BikeOption[]
  advice: string
  warnings?: string[]
}

export interface Plan {
  direction: Direction
  mode: 'live'
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
  weather: { status: 'ok' | 'stale' | 'unavailable'; source?: string; fetched?: string; error?: string }
  bike: BikeView | null
  bike_error?: string
}

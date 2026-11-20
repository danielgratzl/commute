<script lang="ts">
  import type { BikeView, BikeOption, Connection } from '../lib/types'
  import { clock } from '../lib/time'
  import { icon } from '../lib/icons'

  let { b, train, live }: { b: BikeView; train?: Connection; live: boolean } = $props()

  const best = $derived(b.best)
  const w = $derived(best.weather)
  const diff = $derived(train ? train.minutes - b.door_min : null)
  const tone = (o: BikeOption) => (o.verdict === 'ride' ? 'ok' : o.verdict === 'gear' ? 'maybe' : o.verdict === 'unknown' ? 'plain' : 'rain')
  const vIcon = $derived(best.verdict === 'ride' ? 'bike' : best.verdict === 'gear' ? 'umbrella' : best.verdict === 'unknown' ? 'info' : 'train')

  // Elevation profile path in a 300x56 box.
  const profile = $derived.by(() => {
    const p = b.profile
    if (!p?.length) return null
    const W = 300, H = 56, pad = 4
    const maxKm = p[p.length - 1][0] || 1
    const eles = p.map((x) => x[1])
    const lo = Math.min(...eles), hi = Math.max(...eles)
    const span = Math.max(hi - lo, 40)
    const y = (e: number) => H - pad - ((e - lo) / span) * (H - 2 * pad)
    const pts = p.map(([km, e]) => `${((km / maxKm) * W).toFixed(1)},${y(e).toFixed(1)}`)
    return { line: 'M' + pts.join(' L'), area: `M0,${H} L${pts.join(' L')} L${W},${H}Z`, lo: Math.round(lo), hi: Math.round(hi), start: Math.round(p[0][1]), end: Math.round(p[p.length - 1][1]) }
  })
</script>

<section class="card bike">
  <div class="head">
    <div class="bi">{@html icon('bike', 24)}</div>
    <div class="ttl">
      <h3>{b.label}</h3>
      <p>{b.distance_km} km · ↑{b.ascent_m} m ↓{b.descent_m} m · {b.signals} lights</p>
    </div>
    <span class="chip {tone(best)}">{best.verdict === 'ride' ? 'Good to ride' : best.verdict === 'gear' ? 'Rain gear' : best.verdict === 'unknown' ? 'No forecast' : 'Take the train'}</span>
  </div>

  <div class="times">
    <div><b>{b.door_min} min</b><small>door to door</small></div>
    <div><b>{clock(best.leave_at)} → {clock(best.door_arrive)}</b><small>{live ? 'if you leave now' : 'suggested'}</small></div>
    {#if diff !== null}
      <div><b class:faster={diff > 0}>{diff > 0 ? `${diff} min faster` : diff < 0 ? `${-diff} min slower` : 'same time'}</b><small>than the {clock(train!.door_depart)} train</small></div>
    {/if}
  </div>

  <div class="verdict v-{tone(best)}">
    {@html icon(vIcon, 18)}
    <div><b>{best.headline}</b>{#if w}<span>{w.detail} · {Math.round(w.origin.temp)}° → {Math.round(w.destination.temp)}°</span>{/if}</div>
  </div>

  {#if b.options.length}
    <div class="scan" role="list" aria-label="Bike departures and rain">
      {#each b.options as o, i}
        <div class="slot {tone(o)}" class:best={i === b.best_index} role="listitem" title="{clock(o.leave_at)} · {o.headline}{o.weather ? ` · ${o.weather.chance}% rain` : ''}">
          <i></i><span>{i % 2 === 0 || i === b.best_index ? clock(o.leave_at) : ''}</span>
        </div>
      {/each}
    </div>
    {#if b.advice}<p class="advice">{b.advice}</p>{/if}
  {/if}

  {#if b.warnings?.length}
    <div class="warn">
      {#each b.warnings as x}<span class="chip plain">{@html icon(x.startsWith('Dark') ? 'moon' : x.startsWith('Gust') ? 'wind' : 'info', 13)}{x}</span>{/each}
    </div>
  {/if}

  {#if profile}
    <figure class="profile">
      <svg viewBox="0 0 300 56" preserveAspectRatio="none" aria-label="Elevation profile">
        <path d={profile.area} class="area" />
        <path d={profile.line} class="line" vector-effect="non-scaling-stroke" />
      </svg>
      <figcaption><span>{profile.start} m</span><span>elevation · {b.ride_min} min riding</span><span>{profile.end} m</span></figcaption>
    </figure>
  {/if}
</section>

<style>
  .bike { padding: 14px; display: flex; flex-direction: column; gap: 12px; }
  .head { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; gap: 12px; align-items: center; }
  .bi { width: 44px; height: 44px; border-radius: 14px; background: color-mix(in srgb, var(--m-bike) 14%, transparent); color: var(--m-bike); display: grid; place-items: center; }
  h3 { margin: 0; font-size: 16px; font-weight: 750; }
  .ttl p { margin: 2px 0 0; font-size: 12.5px; color: var(--muted); }
  .times { display: flex; gap: 18px; flex-wrap: wrap; }
  .times b { display: block; font-size: 17px; font-weight: 750; }
  .times b.faster { color: var(--go); }
  .times small { font-size: 12px; color: var(--muted); }
  .verdict { display: flex; gap: 10px; align-items: flex-start; padding: 10px 12px; border-radius: 14px; font-size: 13px; line-height: 1.35; }
  .verdict b { display: block; font-size: 14.5px; }
  .verdict :global(svg) { margin-top: 1px; }
  .v-ok { background: var(--go-soft); }
  .v-ok :global(svg) { color: var(--go); }
  .v-maybe { background: var(--maybe-soft); }
  .v-maybe :global(svg) { color: var(--maybe); }
  .v-rain { background: var(--rain-soft); color: var(--rain-ink); }
  .v-plain { background: var(--card2); }
  .scan { display: grid; grid-template-columns: repeat(10, minmax(0, 1fr)); gap: 3px; }
  .slot { display: flex; flex-direction: column; gap: 3px; }
  .slot i { height: 18px; border-radius: 5px; background: var(--card2); }
  .slot.ok i { background: color-mix(in srgb, var(--go) 55%, transparent); }
  .slot.maybe i { background: color-mix(in srgb, var(--maybe) 55%, transparent); }
  .slot.rain i { background: color-mix(in srgb, var(--rain) 60%, transparent); }
  .slot.best i { outline: 2px solid var(--ink); outline-offset: 1px; }
  .slot span { font-size: 10px; color: var(--faint); white-space: nowrap; min-height: 12px; }
  .slot.best span { color: var(--ink); font-weight: 700; }
  .advice { margin: -4px 0 0; font-size: 13px; font-weight: 650; }
  .warn { display: flex; gap: 6px; flex-wrap: wrap; }
  .profile { margin: 0; }
  .profile svg { width: 100%; height: 56px; display: block; }
  .area { fill: color-mix(in srgb, var(--m-bike) 14%, transparent); }
  .line { fill: none; stroke: var(--m-bike); stroke-width: 2; }
  figcaption { display: flex; justify-content: space-between; font-size: 11px; color: var(--faint); margin-top: 3px; }
</style>

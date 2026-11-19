<script lang="ts">
  import type { PointWeather } from '../lib/types'
  import { clock } from '../lib/time'
  import { icon } from '../lib/icons'

  let { label, icon: placeIcon, p }: { label: string; icon: string; p: PointWeather } = $props()

  const sky = $derived(p.snow ? 'snow' : p.chance >= 50 ? 'rain' : 'cloud')
</script>

<div class="tile">
  <div class="where">{@html icon(placeIcon, 14)}<span>{label} · {clock(p.at)}</span></div>
  <div class="row">
    <span class="temp">{Math.round(p.temp)}°</span>
    <span class="sky" class:wet={p.chance >= 50}>{@html icon(sky, 28)}</span>
  </div>
  <div class="pr">
    <span class="chance" class:wet={p.chance >= 20}>{p.chance}% rain</span>
    <span>feels {Math.round(p.feels)}°</span>
  </div>
</div>

<style>
  .tile { background: var(--card2); border-radius: 16px; padding: 12px; min-width: 0; }
  .where { font-size: 12px; color: var(--muted); display: flex; align-items: center; gap: 5px; white-space: nowrap; overflow: hidden; }
  .where span { overflow: hidden; text-overflow: ellipsis; }
  .row { display: flex; align-items: center; justify-content: space-between; margin-top: 6px; }
  .temp { font-size: 30px; font-weight: 750; letter-spacing: -0.03em; }
  .sky { color: var(--muted); }
  .sky.wet { color: var(--rain); }
  .pr { font-size: 12.5px; font-weight: 600; margin-top: 2px; display: flex; gap: 8px; flex-wrap: wrap; color: var(--muted); }
  .chance.wet { color: var(--rain); }
</style>

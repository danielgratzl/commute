<script lang="ts">
  import type { Connection } from '../lib/types'
  import { clock, inWords, ms } from '../lib/time'
  import { icon } from '../lib/icons'

  let { c, now, live, selected, onselect }: { c: Connection; now: number; live: boolean; selected: boolean; onselect: () => void } = $props()

  const inMin = $derived(Math.ceil((ms(c.leave_at) - now) / 60000))
  const rides = $derived(c.legs.filter((l) => l.mode !== 'walk'))
  const cancelled = $derived(c.status === 'cancelled')
</script>

<button class="row" class:sel={selected} class:cancelled disabled={cancelled} onclick={onselect}>
  <div class="lv">
    <b>{clock(c.door_depart)}</b>
    <small>{cancelled ? 'cancelled' : !live ? 'leave' : inMin <= 0 ? 'now' : 'in ' + inWords(inMin)}</small>
  </div>
  <div class="mid">
    <div class="badges">
      {#each rides as l}<span class="badge {l.cancelled ? 'cancelled' : l.mode}">{@html icon(l.mode, 13)}{l.line}</span>{/each}
    </div>
    <div class="meta">
      {#if cancelled}
        {c.status_note}
      {:else}
        → <b>{clock(c.door_arrive)}</b> · <b>{c.minutes} min</b> door to door
      {/if}
    </div>
  </div>
  <div class="st">
    {#if cancelled}
      <span class="chip late">Cancelled</span>
    {:else}
      {#if c.weather}
        <span class="chip {c.weather.verdict === 'wet' ? 'rain' : c.weather.verdict === 'maybe' ? 'maybe' : 'ok'}">
          {@html icon(c.weather.verdict === 'dry' ? 'cloud' : 'umbrella', 13)}{c.weather.verdict === 'dry' ? 'dry' : c.weather.chance + '%'}
        </span>
      {/if}
      <span class="sub">
        {#if c.status === 'at_risk'}<b class="risk">at risk</b>
        {:else if c.delay_min > 0}<b class="late">+{c.delay_min}′</b>
        {:else if c.weather}{Math.round(c.weather.origin.temp)}°→{Math.round(c.weather.destination.temp)}°
        {:else}{c.transfers === 0 ? 'direct' : c.transfers + '×'}{/if}
      </span>
    {/if}
  </div>
</button>

<style>
  .row { display: grid; grid-template-columns: 58px minmax(0, 1fr) auto; gap: 10px; align-items: center; padding: 12px 14px; border: 0; background: none; width: 100%; text-align: left; cursor: pointer; }
  .row + :global(.row) { border-top: 1px solid var(--line); }
  .row:active { background: var(--card2); }
  .row.sel { background: color-mix(in srgb, var(--accent) 9%, transparent); }
  .row.cancelled { cursor: default; opacity: 0.6; }
  .row.cancelled .lv b { text-decoration: line-through; }
  .lv { line-height: 1.15; }
  .lv b { font-size: 19px; font-weight: 750; display: block; }
  .lv small { font-size: 12px; color: var(--muted); }
  .mid { min-width: 0; }
  .badges { display: flex; gap: 4px; overflow: hidden; }
  .meta { font-size: 12.5px; color: var(--muted); margin-top: 5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .st { display: flex; flex-direction: column; align-items: flex-end; gap: 3px; }
  .sub { font-size: 12px; color: var(--muted); font-weight: 600; }
  .sub .risk { color: var(--warn); }
  .sub .late { color: var(--late); }
  .meta b { color: var(--ink); font-weight: 650; }
</style>

<script lang="ts">
  import type { Connection, OutdoorInterval } from '../lib/types'
  import { ms } from '../lib/time'
  import { icon } from '../lib/icons'

  let { c }: { c: Connection } = $props()

  type Seg = { kind: string; line?: string; dur: number; wx?: OutdoorInterval }

  const intervals = $derived(c.weather?.intervals ?? [])
  const walkWx = (i: number) => intervals.find((iv) => iv.kind === 'walk' && iv.leg === i)
  const waitWx = (i: number) => intervals.find((iv) => iv.kind === 'wait' && iv.leg === i)

  // Leg segments plus waits, proportional to time, so the bar reads like a timeline.
  const segs = $derived.by(() => {
    const out: Seg[] = []
    let cur = ms(c.legs[0].depart)
    c.legs.forEach((l, i) => {
      const dep = ms(l.depart)
      const arr = ms(l.arrive)
      if (dep > cur) out.push({ kind: 'wait', dur: (dep - cur) / 60000, wx: waitWx(i - 1) })
      const start = Math.max(dep, cur)
      out.push({
        kind: l.cancelled ? 'cancelled' : l.mode,
        line: l.line,
        dur: Math.max(1, l.mode === 'walk' ? l.minutes : (arr - start) / 60000),
        wx: l.mode === 'walk' ? walkWx(i) : undefined,
      })
      cur = Math.max(cur, l.mode === 'walk' ? start + l.minutes * 60000 : arr)
    })
    return out
  })

  // Rain strength for an outdoor stretch: 0 (dry) .. 1 (certain rain).
  const wet = (s: Seg) => (s.wx && s.wx.chance >= 20 ? s.wx.chance / 100 : 0)
</script>

<div class="legs" aria-label="Trip legs">
  {#each segs as s}
    {#if s.kind === 'wait'}
      <div class="seg wait" class:wet={wet(s) > 0} style="flex:{s.dur};--w:{wet(s)}" title={s.wx ? `Wait outside · ${s.wx.chance}% rain` : 'Wait'}></div>
    {:else}
      <div
        class="seg {s.kind}"
        class:wet={wet(s) > 0}
        style="flex:{s.dur};--w:{wet(s)}"
        title={s.wx ? `Walk ${Math.round(s.dur)} min · ${s.wx.chance}% rain` : undefined}
      >
        {@html icon(s.kind === 'cancelled' ? 'warn' : wet(s) > 0 && s.kind === 'walk' ? 'drop' : s.kind, 15)}
        {#if s.line && s.dur >= 5}<span>{s.line}</span>{/if}
      </div>
    {/if}
  {/each}
</div>
<div class="cap">
  <span><i class="k walk"></i>outside, dry</span>
  <span><i class="k rain"></i>outside, rain likely</span>
</div>

<style>
  .legs { display: flex; gap: 3px; }
  .seg {
    height: 30px; border-radius: 8px; display: flex; align-items: center; justify-content: center;
    gap: 4px; color: #fff; font-size: 12px; font-weight: 750; min-width: 28px; overflow: hidden; white-space: nowrap;
  }
  .seg.walk { background: transparent; color: var(--muted); border: 1.5px dashed var(--m-walk); min-width: 26px; }
  .seg.walk.wet {
    border-style: solid; border-color: var(--rain); color: var(--rain-ink);
    background: color-mix(in srgb, var(--rain) calc(var(--w) * 45%), transparent);
  }
  .seg.wait { background: transparent; min-width: 4px; position: relative; }
  .seg.wait::after { content: ''; position: absolute; left: 0; right: 0; top: 50%; border-top: 2px dotted var(--line); }
  .seg.wait.wet { background: color-mix(in srgb, var(--rain) calc(var(--w) * 45%), transparent); border-radius: 4px; }
  .seg.wait.wet::after { border-color: var(--rain); }
  .seg.bus { background: var(--m-bus); }
  .seg.train { background: var(--m-train); }
  .seg.tram { background: var(--m-tram); }
  .seg.ship, .seg.cable, .seg.other { background: var(--m-other); }
  .seg.cancelled { background: var(--late-soft); color: var(--late); }
  .cap { display: flex; gap: 14px; margin-top: 6px; font-size: 11px; color: var(--faint); }
  .cap span { display: inline-flex; align-items: center; gap: 5px; }
  .k { width: 12px; height: 8px; border-radius: 2px; display: inline-block; }
  .k.walk { border: 1.5px dashed var(--m-walk); }
  .k.rain { background: color-mix(in srgb, var(--rain) 45%, transparent); border: 1.5px solid var(--rain); }
</style>

<script lang="ts">
  import type { Connection } from '../lib/types'
  import { ms } from '../lib/time'
  import { icon } from '../lib/icons'

  let { c }: { c: Connection } = $props()

  type Seg = { kind: string; line?: string; dur: number }

  // Leg segments plus waits, proportional to time, so the bar reads like a timeline.
  const segs = $derived.by(() => {
    const out: Seg[] = []
    let cur = ms(c.legs[0].depart)
    for (const l of c.legs) {
      const dep = ms(l.depart)
      const arr = ms(l.arrive)
      if (dep > cur) out.push({ kind: 'wait', dur: (dep - cur) / 60000 })
      const start = Math.max(dep, cur)
      out.push({
        kind: l.cancelled ? 'cancelled' : l.mode,
        line: l.line,
        dur: Math.max(1, l.mode === 'walk' ? l.minutes : (arr - start) / 60000),
      })
      cur = Math.max(cur, l.mode === 'walk' ? start + l.minutes * 60000 : arr)
    }
    return out
  })
</script>

<div class="legs" aria-label="Trip legs">
  {#each segs as s}
    {#if s.kind === 'wait'}
      <div class="seg wait" style="flex:{s.dur}"></div>
    {:else}
      <div class="seg {s.kind}" style="flex:{s.dur}">
        {@html icon(s.kind === 'cancelled' ? 'warn' : s.kind, 15)}
        {#if s.line && s.dur >= 5}<span>{s.line}</span>{/if}
      </div>
    {/if}
  {/each}
</div>

<style>
  .legs { display: flex; gap: 3px; }
  .seg {
    height: 30px; border-radius: 8px; display: flex; align-items: center; justify-content: center;
    gap: 4px; color: #fff; font-size: 12px; font-weight: 750; min-width: 28px; overflow: hidden; white-space: nowrap;
  }
  .seg.walk { background: transparent; color: var(--muted); border: 1.5px dashed var(--m-walk); min-width: 26px; }
  .seg.wait { background: transparent; min-width: 4px; position: relative; }
  .seg.wait::after { content: ''; position: absolute; left: 0; right: 0; top: 50%; border-top: 2px dotted var(--line); }
  .seg.bus { background: var(--m-bus); }
  .seg.train { background: var(--m-train); }
  .seg.tram { background: var(--m-tram); }
  .seg.ship, .seg.cable, .seg.other { background: var(--m-other); }
  .seg.cancelled { background: var(--late-soft); color: var(--late); }
</style>

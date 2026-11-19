<script lang="ts">
  import type { Connection, Leg, Plan } from '../lib/types'
  import { clock, inWords, mmss, ms } from '../lib/time'
  import { icon } from '../lib/icons'
  import LegBar from './LegBar.svelte'

  let {
    c,
    plan,
    now,
    from,
    to,
    pinned,
    onunpin,
  }: { c: Connection; plan: Plan; now: number; from: string; to: string; pinned: boolean; onunpin: () => void } = $props()

  const secs = $derived((ms(c.leave_at) - now) / 1000)
  const mins = $derived(Math.max(0, Math.ceil(secs / 60)))
  const live = $derived(plan.mode === 'live')
  const go = $derived(live && secs <= 120)
  const progress = $derived(Math.max(0, Math.min(100, 100 - (secs / (20 * 60)) * 100)))
  const firstStop = $derived(c.legs.find((l) => l.mode !== 'walk')?.from ?? '')

  const lineLabel = (l: Leg) =>
    l.mode === 'bus' ? `Bus ${l.line}` : l.mode === 'tram' ? `Tram ${l.line}` : (l.line ?? '')
  const depWithDelay = (l: Leg) => clock(ms(l.depart) + (l.dep_delay && l.dep_delay > 0 ? l.dep_delay * 60000 : 0))
</script>

<article class="hero" class:go aria-live="polite">
  <div class="head">
    <div class="eyebrow">
      <span>{pinned ? 'Selected connection' : live ? 'Next connection' : 'Suggested connection'}</span>
      <span class="chips">
        {#if pinned}<button class="chip plain" onclick={onunpin}>Show best</button>{/if}
        {#if c.status === 'at_risk'}
          <span class="chip risk">{@html icon('warn', 13)} At risk</span>
        {:else if c.delay_min > 0}
          <span class="chip late">{@html icon('warn', 13)} +{c.delay_min}′</span>
        {:else if c.has_realtime}
          <span class="chip ok">On time</span>
        {:else}
          <span class="chip plain">Timetable</span>
        {/if}
      </span>
    </div>

    <div class="count">
      <div>
        {#if live && secs < 90 * 60}
          <div class="lbl">{go ? 'Go now' : 'Leave in'}</div>
          <div class="big">
            {#if go}<span class="n">{mmss(secs)}</span>{:else}<span class="n">{mins}</span><span class="u">min</span>{/if}
          </div>
        {:else}
          <div class="lbl">Out the door</div>
          <div class="big"><span class="n">{clock(c.door_depart)}</span></div>
        {/if}
      </div>
      <div class="side">
        {#if live && secs < 90 * 60}
          <span>out the door</span><b>{clock(c.door_depart)}</b>
        {:else}
          <span>{plan.mode === 'planned' ? plan.label.split(',')[0] : 'in'}</span><b>{secs > 0 ? 'in ' + inWords(mins) : 'now'}</b>
        {/if}
        <span class="sub">{go ? 'walk to ' + firstStop : ''}</span>
      </div>
    </div>
    {#if live}<div class="ring"><i style="width:{progress}%"></i></div>{/if}
  </div>

  {#if c.status === 'at_risk' && c.status_note}
    <div class="alert">{@html icon('warn', 16)}<div>{c.status_note}</div></div>
  {/if}

  <div class="trip">
    <div class="ends">
      <div class="t">{clock(c.door_depart)}<small>{from}</small></div>
      <div class="mid"><span><b>{c.minutes} min</b>door to door</span></div>
      <div class="t r">
        <span class:del={c.delay_min > 0}>{clock(c.door_arrive)}</span>
        <small>{#if c.delay_min > 0}<s>{clock(c.planned_arrive)}</s> {/if}{to}</small>
      </div>
    </div>
  </div>

  <div class="bar"><LegBar {c} /></div>

  <details class="steps">
    <summary>All {c.legs.length} steps{c.transfers ? ` · ${c.transfers} change${c.transfers > 1 ? 's' : ''}` : ''} {@html icon('chev', 16)}</summary>
    {#each c.legs as l, i}
      <div class="step">
        <div class="tm">
          {l.mode === 'walk' ? clock(l.depart) : depWithDelay(l)}
          {#if l.dep_delay && l.dep_delay > 0}<small class="del">+{l.dep_delay}′</small>{/if}
        </div>
        <div class="dot"><b class={l.cancelled ? 'cancelled' : l.mode}>{@html icon(l.mode, 14)}</b>{#if i < c.legs.length - 1}<em></em>{/if}</div>
        <div class="tx">
          {#if l.mode === 'walk'}
            Walk {l.minutes} min to <b>{l.to}</b>
          {:else}
            <b class:strike={l.cancelled}>{lineLabel(l)}</b> → {l.direction}
            {#if l.cancelled}<span class="del"> cancelled</span>{/if}
            <small>
              {l.from}{#if l.track} · Pl. <span class:changed={l.track_changed}>{l.track}</span>{/if} → {l.to} · arr {clock(ms(l.arrive) + (l.arr_delay ?? 0) * 60000)}
            </small>
          {/if}
        </div>
      </div>
    {/each}
    {#if c.disruptions?.length}
      <div class="disr">
        {#each c.disruptions as d}<p>{@html icon('info', 14)}<span>{d}</span></p>{/each}
      </div>
    {/if}
  </details>
</article>

<style>
  .hero { background: var(--card); border-radius: 28px; box-shadow: var(--shadow); overflow: hidden; }
  .hero.go { box-shadow: 0 0 0 2px var(--warn), var(--shadow); }
  .head { padding: 18px 20px 6px; position: relative; }
  .eyebrow { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); }
  .chips { display: flex; gap: 6px; text-transform: none; letter-spacing: 0; }
  .chips button { border: 0; cursor: pointer; }
  .count { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; margin: 6px 0 14px; }
  .lbl { font-size: 17px; font-weight: 600; color: var(--muted); margin-bottom: 2px; }
  .big { display: flex; align-items: baseline; gap: 6px; line-height: 0.9; }
  .big .n { font-size: clamp(64px, 22vw, 92px); font-weight: 800; letter-spacing: -0.045em; }
  .big .u { font-size: 26px; font-weight: 700; color: var(--muted); }
  .go .lbl, .go .big .n { color: var(--warn); }
  .side { text-align: right; display: flex; flex-direction: column; font-size: 13px; color: var(--muted); padding-bottom: 6px; }
  .side b { font-size: 22px; color: var(--ink); font-weight: 750; }
  .side .sub { font-weight: 600; min-height: 1em; }
  .ring { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: var(--line); }
  .ring i { display: block; height: 100%; background: var(--accent); transition: width 1s linear; }
  .go .ring i { background: var(--warn); }
  .alert { margin: 12px 14px 0; padding: 10px 12px; border-radius: 14px; background: var(--warn-soft); font-size: 13px; line-height: 1.35; display: flex; gap: 10px; align-items: flex-start; color: var(--ink); }
  .alert :global(svg) { color: var(--warn); }
  .trip { padding: 16px 20px 10px; }
  .ends { display: grid; grid-template-columns: auto minmax(90px, 1fr) auto; align-items: center; gap: 8px; }
  .t { font-size: 22px; font-weight: 750; }
  .t small { display: block; font-size: 12px; font-weight: 550; color: var(--muted); }
  .t.r { text-align: right; }
  .mid { text-align: center; font-size: 12px; color: var(--muted); position: relative; }
  .mid b { display: block; font-size: 15px; color: var(--ink); }
  .mid::before { content: ''; position: absolute; left: 0; right: 0; top: 50%; border-top: 1.5px dashed var(--line); }
  .mid span { position: relative; background: var(--card); padding: 0 6px; display: inline-block; white-space: nowrap; }
  .del { color: var(--late); font-weight: 700; }
  s { color: var(--faint); }
  .bar { padding: 4px 20px 6px; }
  .steps { margin: 8px 14px 14px; border-top: 1px solid var(--line); }
  summary { list-style: none; cursor: pointer; padding: 12px 6px 4px; font-size: 14px; font-weight: 650; color: var(--accent); display: flex; justify-content: space-between; align-items: center; }
  summary::-webkit-details-marker { display: none; }
  details[open] summary :global(svg) { transform: rotate(180deg); }
  .step { display: grid; grid-template-columns: 46px 22px minmax(0, 1fr); gap: 8px; padding: 8px 6px 0; font-size: 14px; }
  .tm { font-weight: 700; text-align: right; line-height: 1.2; }
  .tm small { display: block; font-size: 11px; }
  .dot { display: flex; flex-direction: column; align-items: center; }
  .dot b { width: 22px; height: 22px; border-radius: 7px; display: grid; place-items: center; color: #fff; }
  .dot b.walk { background: var(--card2); color: var(--muted); }
  .dot b.bus { background: var(--m-bus); }
  .dot b.train { background: var(--m-train); }
  .dot b.tram { background: var(--m-tram); }
  .dot b.ship, .dot b.cable, .dot b.other { background: var(--m-other); }
  .dot b.cancelled { background: var(--late); }
  .dot em { flex: 1; width: 2px; background: var(--line); margin: 3px 0 -6px; min-height: 14px; }
  .tx { line-height: 1.3; padding-bottom: 8px; min-width: 0; }
  .tx small { display: block; color: var(--muted); font-size: 12px; }
  .strike { text-decoration: line-through; }
  .changed { color: var(--late); font-weight: 700; }
  .disr { margin: 6px 6px 0; display: flex; flex-direction: column; gap: 6px; }
  .disr p { margin: 0; display: flex; gap: 8px; font-size: 12.5px; color: var(--muted); line-height: 1.35; }
</style>

<script lang="ts">
  import { onMount } from 'svelte'
  import type { Direction, LiveResponse } from './lib/types'
  import { fetchLive, type Override } from './lib/api'
  import { ago, clock, ms } from './lib/time'
  import { icon } from './lib/icons'
  import Hero from './components/Hero.svelte'
  import Row from './components/Row.svelte'
  import BikeCard from './components/BikeCard.svelte'

  const REFRESH_MS = 60_000

  let data = $state<LiveResponse | null>(null)
  let error = $state<string | null>(null)
  let loading = $state(false)
  let override = $state<Override>({})
  let pinnedId = $state<string | null>(null)
  let now = $state(Date.now())
  let loadedAt = $state(0)
  let refetchedFor = ''
  let inflight: AbortController | null = null

  async function load() {
    inflight?.abort()
    const ctl = new AbortController()
    inflight = ctl
    loading = true
    try {
      data = await fetchLive(override, ctl.signal)
      error = null
      loadedAt = Date.now()
    } catch (e) {
      if ((e as Error).name !== 'AbortError') error = (e as Error).message
    } finally {
      if (inflight === ctl) loading = false
    }
  }

  const live = $derived(data?.plan.mode === 'live')
  const visible = $derived.by(() => {
    if (!data) return []
    // Hide connections that have left (the server filters too, but the page may sit open).
    return data.connections.filter((c) => !live || ms(c.leave_at) > now - 60_000)
  })
  const pinned = $derived(visible.find((c) => c.id === pinnedId && c.status !== 'cancelled' && ms(c.leave_at) > now - 30_000))
  const best = $derived(visible.find((c) => c.id === data?.hero_id))
  const hero = $derived(pinned ?? best)
  const later = $derived(visible.filter((c) => c.id !== hero?.id).slice(0, 6))

  function toggleDirection() {
    if (!data) return
    const next: Direction = data.plan.direction === 'h2w' ? 'w2h' : 'h2w'
    override = { dir: next }
    pinnedId = null
    load()
  }
  function backToAuto() {
    override = {}
    pinnedId = null
    load()
  }
  function select(id: string) {
    pinnedId = id === data?.hero_id ? null : id
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  onMount(() => {
    load()
    const tick = setInterval(() => {
      now = Date.now()
      // When the shown connection has left, ask the server for the next one (served from cache).
      if (live && best && ms(best.leave_at) < now - 30_000 && refetchedFor !== best.id) {
        refetchedFor = best.id
        load()
      }
    }, 1000)
    const poll = setInterval(() => {
      if (document.visibilityState === 'visible') load()
    }, REFRESH_MS)
    const onVis = () => {
      if (document.visibilityState === 'visible' && Date.now() - loadedAt > 15_000) load()
    }
    document.addEventListener('visibilitychange', onVis)
    return () => {
      clearInterval(tick)
      clearInterval(poll)
      document.removeEventListener('visibilitychange', onVis)
    }
  })
</script>

<div class="app">
  <header class="top">
    <div class="dir">
      <button class="dir-btn" onclick={toggleDirection} disabled={!data} aria-label="Switch direction">
        <span class="sw">{@html icon('swap', 15)}</span>
        <span>{data ? `${data.from} → ${data.to}` : 'Commute'}</span>
      </button>
      {#if data}
        <div class="dir-sub">
          {#if data.plan.auto}
            Auto · {data.plan.reason}
          {:else}
            {data.plan.reason} · <button class="link" onclick={backToAuto}>back to auto</button>
          {/if}
        </div>
      {/if}
    </div>
    <div class="clock">
      <b>{clock(now)}</b>
      <span class="live" class:stale={data?.stale || error}><i></i>{loadedAt ? ago((now - loadedAt) / 1000) : '…'}</span>
    </div>
  </header>

  <main>
    {#if data?.plan.commute_day}
      <div class="context"><span class="chip ok">Commute day</span></div>
    {/if}

    {#if data && data.weather.status === 'unavailable'}
      <div class="banner">{@html icon('warn', 16)}<span>Weather unavailable right now. Connections are still live.</span></div>
    {/if}

    {#if data?.warning}
      <div class="banner">{@html icon('warn', 16)}<span>{data.warning}</span></div>
    {/if}

    {#if error && !data}
      <div class="card empty">
        <b>Can't load connections</b>
        <p>{error}</p>
        <button class="retry" onclick={load}>Try again</button>
      </div>
    {:else if !data}
      <div class="card empty"><b>Loading connections…</b></div>
    {:else if hero}
      <Hero c={hero} plan={data.plan} {now} from={data.from} to={data.to} pinned={!!pinned} onunpin={() => (pinnedId = null)} />
    {:else}
      <div class="card empty">
        <b>No connection to suggest</b>
        <p>Everything in this window has left or is cancelled. Switch direction or wait for the next refresh.</p>
      </div>
    {/if}

    {#if error && data}
      <div class="banner">{@html icon('warn', 16)}<span>Refresh failed: {error}. Showing data from {clock(data.fetched_at)}.</span></div>
    {/if}

    {#if later.length}
      <div class="sec-h"><h2>Later</h2><span>door to door</span></div>
      <div class="card list">
        {#each later as c (c.id)}
          <Row {c} {now} {live} selected={c.id === pinnedId} onselect={() => select(c.id)} />
        {/each}
      </div>
    {/if}

    {#if data?.bike}
      <div class="sec-h"><h2>By bike instead?</h2><span>{data.bike.ride_min} min riding</span></div>
      <BikeCard b={data.bike} train={best} {live} />
    {:else if data?.bike_error}
      <div class="sec-h"><h2>By bike instead?</h2></div>
      <div class="card empty"><p>{data.bike_error}</p></div>
    {/if}
  </main>

  <footer class="foot">
    {#if data}
      Timetable: search.ch, fetched {clock(data.fetched_at)} · {data.budget.used}/{data.budget.limit} requests today<br />
      {#if data.weather.status !== 'unavailable'}
        Weather: <a href="https://open-meteo.com/" target="_blank" rel="noopener">Open-Meteo.com</a> (CC BY 4.0), source MeteoSwiss{#if data.weather.fetched}, fetched {clock(data.weather.fetched)}{/if}<br />
      {/if}
      Bike route: BRouter, © OpenStreetMap contributors<br />
      Times include walking from and to the door{#if loading} · refreshing…{/if}
    {/if}
  </footer>
</div>

<style>
  .app { max-width: 460px; margin: 0 auto; padding: max(12px, env(safe-area-inset-top)) max(16px, env(safe-area-inset-right)) max(28px, env(safe-area-inset-bottom)) max(16px, env(safe-area-inset-left)); }
  .top { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 6px 2px 12px; }
  .dir { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .dir-btn { align-self: flex-start; white-space: nowrap; display: flex; align-items: center; gap: 8px; border: 0; background: var(--card); box-shadow: var(--shadow); border-radius: 999px; padding: 8px 14px 8px 8px; font-weight: 650; font-size: 15px; cursor: pointer; }
  .sw { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; background: var(--accent); color: var(--accent-ink); }
  .dir-sub { font-size: 12px; color: var(--muted); padding-left: 4px; }
  .link { border: 0; background: none; padding: 0; color: var(--accent); font-weight: 600; font-size: inherit; cursor: pointer; text-decoration: underline; text-underline-offset: 2px; }
  .clock { text-align: right; font-size: 12px; color: var(--muted); white-space: nowrap; }
  .clock b { display: block; font-size: 17px; color: var(--ink); font-weight: 700; }
  .live { display: inline-flex; align-items: center; gap: 5px; }
  .live i { width: 7px; height: 7px; border-radius: 50%; background: var(--go); }
  .live.stale i { background: var(--warn); }
  .context { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin: 0 2px 12px; font-size: 13px; }
  .banner { display: flex; gap: 10px; align-items: flex-start; margin: 0 0 12px; padding: 10px 12px; border-radius: 14px; background: var(--warn-soft); font-size: 13px; line-height: 1.35; }
  .banner :global(svg) { color: var(--warn); margin-top: 1px; }
  .card { background: var(--card); border-radius: var(--r); box-shadow: var(--shadow); overflow: hidden; }
  .empty { padding: 28px 20px; text-align: center; color: var(--muted); }
  .empty b { display: block; font-size: 19px; color: var(--ink); margin-bottom: 6px; }
  .empty p { margin: 0 0 12px; font-size: 14px; }
  .retry { border: 0; background: var(--accent); color: var(--accent-ink); border-radius: 999px; padding: 8px 16px; font-weight: 650; cursor: pointer; }
  .sec-h { display: flex; align-items: baseline; justify-content: space-between; margin: 22px 4px 10px; }
  .sec-h h2 { margin: 0; font-size: 17px; font-weight: 750; }
  .sec-h span { font-size: 12px; color: var(--muted); }
  .foot a { color: inherit; }
  .foot { margin-top: 22px; text-align: center; font-size: 12px; color: var(--faint); line-height: 1.6; }
</style>

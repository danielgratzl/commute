# Commute

A self-hosted phone app (PWA) that shows the next sensible public-transport connection between home and work, door to door, with live delays. Weather and bike comes in later phases.

- **Backend:** Go, single binary, serves the API and the embedded PWA.
- **Frontend:** Svelte 5 + Vite, installable PWA.
- **Data:** [search.ch timetable API](https://search.ch/timetable/api/help) (no key, ~1000 route searches per day).

## How the live view decides what to show

With the default schedule (arrive 09:00, leave 16:00):

| Time | Shows |
|---|---|
| until 10:00 | trip to work, live departures |
| 10:00–15:15 | trip home, planned around 16:00 ("Show departures now" switches to live) |
| 15:15–20:00 | trip home, live departures |
| after 20:00 | tomorrow's trip to work, arriving by 09:00 |

The highlighted connection is the earliest door-to-door arrival you can still catch. Cancelled connections are never highlighted; connections with a transfer at risk count 5 minutes later. Tue/Thu are labelled as commute days, but the app works the same every day. Tap the direction button to switch manually.

## Configuration

```sh
cp config.example.yaml config.yaml   # then set your coordinates and times
```

`config.yaml` is git-ignored because it holds your home coordinates. Use exact coordinates (for example from map.geo.admin.ch); free-text addresses are often geocoded wrong.

## Run locally

```sh
make test     # Go tests (on recorded search.ch responses in testdata/) + svelte-check
make run      # builds the PWA and the binary, serves on :8080
make dev      # API on :8080 + Vite dev server with hot reload on :5173
```

## Deploy on the NAS

```sh
# copy the repo (or later just docker-compose.yml) and your config.yaml to the NAS, then:
docker compose up -d --build
```

The image is about 17 MB and uses about 12 MB RAM. It builds for amd64 and arm64 (`docker buildx build --platform linux/amd64,linux/arm64 .`). The daily request counter lives in the `commute-data` volume.

### HTTPS and access from the office (Tailscale)

The phone can only install the PWA over HTTPS. With Tailscale on the NAS and phone:

```sh
tailscale serve --bg --https=443 http://localhost:8080
```

Then open `https://<nas-name>.<tailnet>.ts.net` on the phone and choose "Add to Home Screen". The app has no login, so keep port 8080 closed on your router.

## Layout

```
main.go                  server + healthcheck subcommand
internal/config          YAML config and defaults
internal/transit         search.ch client and normaliser (door-to-door, delays, cancellations)
internal/engine          direction rules and hero selection
internal/store           response cache and persisted daily request budget
internal/api             HTTP handlers, static PWA
web/                     Svelte PWA (built into web/dist and embedded)
testdata/                recorded search.ch responses for tests
```

## Roadmap

1. ~~Skeleton, live transit view~~
2. Trip weather: Open-Meteo (MeteoSwiss ICON-CH) + ensemble, rain chance during the minutes outdoors
3. Robustness: delay refresh via departure boards, fallback provider, tomorrow view polish
4. Road bike: cached BRouter route, "dry if you leave by" scan
5. CI: GitHub Actions multi-arch image to GHCR

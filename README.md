# Commute

A self-hosted phone app (PWA) that shows the next sensible public-transport connection between home and work, door to door, with live delays and the chance of rain for the minutes you are outside, plus the same trip by road bike.

- **Backend:** Go, single binary, serves the API and the embedded PWA.
- **Frontend:** Svelte 5 + Vite, installable PWA.
- **Data:** [search.ch timetable API](https://search.ch/timetable/api/help) (no key, ~1000 route searches per day), [Open-Meteo](https://open-meteo.com/) with the MeteoSwiss ICON-CH models (no key; weather data CC BY 4.0) and [BRouter](https://brouter.de) for the bike route (OpenStreetMap data).

## How the live view decides what to show

Departures are always live, from now. Before 09:00 the app shows the trip to work, from 09:00 the trip home (`schedule.switch_to_home`). Tap the direction button to switch manually; "back to auto" returns to the time rule. Tue/Thu are labelled as commute days, but the app works the same every day.

For each connection the app works out the minutes you are outside (walks and waits at stops) and the chance of rain during exactly those minutes, from the 11 members of the MeteoSwiss ICON-CH1 ensemble. Below 20% it says no umbrella, from 50% take an umbrella. It also shows temperature and feels-like at both ends.

### Road bike

The bike route comes from [BRouter](https://brouter.de) (profile `fastbike`, asphalt only) and is fetched once per direction, then kept in the data volume. Ride time is not BRouter's estimate: a physics model is calibrated to your `flat_speed_kmh`, slows you down on climbs (the way home climbs ~180 m), caps speed on residential streets, cycle paths and descents, and adds an average wait per traffic light. If your real rides differ, set `k_cal` (1.1 = 10% slower).

The bike card shows door-to-door time, the difference to the suggested train, and the rain verdict for the whole ride (sampled along the route at home, halfway and at work): good to ride, pack rain gear, or take the train. It scans ten departures 10 min apart, starting now, and says when you would stay dry, and warns about darkness, gusts and cold.

The highlighted connection is the earliest door-to-door arrival you can still catch. Cancelled connections are never highlighted; connections with a transfer at risk count 5 minutes later.

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

The image is about 17 MB and uses about 12 MB RAM. It builds for amd64 and arm64 (`docker buildx build --platform linux/amd64,linux/arm64 .`). The daily request counter and the cached bike routes live in the `commute-data` volume.

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
internal/weather         Open-Meteo client, rain chance for the minutes outside
internal/bike            BRouter route, ride time model, bike verdicts
internal/api             HTTP handlers, static PWA
web/                     Svelte PWA (built into web/dist and embedded)
testdata/                recorded search.ch and Open-Meteo responses for tests
```

## Roadmap

1. ~~Skeleton, live transit view~~
2. ~~Trip weather: Open-Meteo (MeteoSwiss ICON-CH) + ensemble, rain chance during the minutes outdoors~~
3. Robustness: delay refresh via departure boards, fallback provider, tomorrow view polish
4. ~~Road bike: cached BRouter route, "dry if you leave by" scan~~
5. CI: GitHub Actions multi-arch image to GHCR

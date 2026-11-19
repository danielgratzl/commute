.PHONY: web build test run dev docker

web:            ## build the PWA into web/dist
	cd web && npm ci && npm run build

build: web     ## single binary with the PWA embedded
	CGO_ENABLED=0 go build -trimpath -o commute .

test:
	go test ./...
	cd web && npx svelte-check --tsconfig ./tsconfig.json

run: build     ## run locally on :8080 with ./config.yaml
	DATA_DIR=./data ./commute

dev:            ## API on :8080 + Vite dev server on :5173 (proxying /api)
	(DATA_DIR=./data go run . &) && cd web && npm run dev

docker:
	docker compose up --build -d

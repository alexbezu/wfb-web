.PHONY: all build frontend run

all: build

frontend:
	cd web && npm ci && npm run build

build:
	go build -o bin/wfb-web ./cmd/wfb-web

run:
	WFB_WEB_CFG=./tmp/wifibroadcast.cfg WFB_WEB_DEFAULT=./tmp/wifibroadcast.default bin/wfb-web

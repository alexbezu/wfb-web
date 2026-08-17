# WFB Web

Minimal WFB-ng ground-station web sidecar.

Install WFB-ng first: https://github.com/svpcom/wfb-ng

Then install this sidecar as a helper service on the ground station.

## Run on SBC

For Raspberry Pi, Radxa, Orange Pi, and similar boards:

```sh
git clone https://github.com/alexbezu/wfb-web.git
cd wfb-web
```

Then use option 2 below. It should work without npm installation 

## Build

### Option 1: Build everything on SBC

You need:

```sh
sudo apt install golang nodejs npm make
```

Then:

```sh
make frontend
make build
```

This builds:

```text
bin/wfb-web
```

### Option 2: Build only Go on SBC

```sh
sudo apt install golang make
make build
```

The Go binary embeds `internal/frontend/dist`, so Node is not needed unless you change `web/src/*`.

## Installation

```sh
make build
sudo install -m 0755 bin/wfb-web /usr/bin/wfb-web
sudo install -m 0644 scripts/default/wfb-web /etc/default/wfb-web
sudo install -m 0644 scripts/systemd/wfb-web.service /lib/systemd/system/wfb-web.service
sudo systemctl daemon-reload
sudo systemctl enable --now wfb-web
```

Then open:

```text
http://SBC_IP:8080/app/
```

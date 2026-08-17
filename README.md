# WFB Web

Minimal WFB-ng ground-station web sidecar.

## Run Locally

```sh
make run
```

The local target uses `./tmp/wifibroadcast.cfg` and `./tmp/wifibroadcast.default`.
On an SBC, install the binary as `/usr/bin/wfb-web`, install
`scripts/default/wfb-web` to `/etc/default/wfb-web`, and install
`scripts/systemd/wfb-web.service` to `/lib/systemd/system/wfb-web.service`.

## Endpoints

- `GET /api/config`
- `GET /api/config/effective`
- `PUT /api/config`
- `GET /api/services`
- `POST /api/services/{wifibroadcast-gs|rtsp-h265|rtsp-h264}/{start|stop|restart|enable|disable}`
- `GET /api/radio`
- `GET /api/stats/stream`

## Build

### Option 1: Build everything on SBC
  You need:

  `sudo apt install golang nodejs npm make`

  Then:
```sh
  cd wfb-web
  make frontend
  make build
```
  This builds:

  `bin/wfb-web`

###  Option 2: Build only Go on SBC
  If you copy the repo with internal/frontend/dist already generated, then the SBC only needs Go:
```sh
  sudo apt install golang make
  cd wfb-web
  make build
```
  The Go binary embeds internal/frontend/dist, so Node is not needed unless you change web/src/*.

  Best SBC deployment shape

  Build binary:
```sh
  make build
  sudo install -m 0755 bin/wfb-web /usr/bin/wfb-web
  sudo install -m 0644 scripts/default/wfb-web /etc/default/wfb-web
  sudo install -m 0644 scripts/systemd/wfb-web.service /lib/systemd/system/wfb-web.service
  sudo systemctl daemon-reload
  sudo systemctl enable --now wfb-web
```
  Then open:

  http://SBC_IP:8080/app/

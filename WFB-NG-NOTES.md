# WFB-ng GS notes
===============

### First install
-------------

```sh
curl -o install_gs.sh https://raw.githubusercontent.com/svpcom/wfb-ng/refs/heads/master/scripts/install_gs.sh
sudo bash ./install_gs.sh
```

Or install from the sources (after git clone):

```sh
  sudo apt install python3-all libpcap-dev libsodium-dev libevent-dev python3-pip \
    python3-pyroute2 python3-twisted python3-serial python3-all-dev python3-venv \
    iw socat debhelper dh-python fakeroot build-essential python3-msgpack \
    python3-setuptools libgstrtspserver-1.0-dev libcatch2-dev -y

  make deb
  sudo apt install ./deb_dist/*.deb
  sudo systemctl daemon-reload
  sudo systemctl start wifibroadcast@gs
```

For the Orange Pi running WFB-ng as the ground station proxy, put this in
`/etc/wifibroadcast.cfg`:

```ini
[common]
wifi_channel = 161
wifi_region = 'BO'
link_domain = 'default'

[base]
ldpc = 0
stbc = 0
bandwidth = 20
mcs_index = 1
force_vht = False

[gs_video]
peer = 'connect://192.168.50.1:5600'       # unicast to PC
#peer = 'connect://239.50.50.50:5600'      # multicast
#peer = 'connect://127.0.0.1:5600'         # local RTSP server
```

or `sudo cp wfb_ng/conf/master.cfg /etc/wifibroadcast.cfg` for full parameter list

`ldpc = 0` belongs in `[base]`. Set it on the TX side first when testing unknown RX hardware; RX receives session/FEC info from the air.
`[gs] streams = ...` overrides the default `master.cfg` GS profile, which otherwise starts video, MAVLink, and tunnel. Keep it here for video-only GS.

Set the WiFi adapter used by WFB-ng:

```sh
sudo nano /etc/default/wifibroadcast
# example:
WFB_NICS="wlan0"

# wfb_rtsp (rtsp@h265 service) settings params (see Option 1: RTSP section)
RTP_MTU=1400
RTP_JITTER=0
RTSP_PORT=8554
RTSP_URI="/wfb"
```

Start simple video-only GS:

```sh
sudo systemctl enable --now wifibroadcast@gs
# or manually:
sudo /usr/bin/wfb-server --profiles gs --wlans wlan0
```

### RTSP / UDP output
-----------------

#### Option 1: RTSP on Orange Pi

Use:

```ini
[gs_video]
peer = 'connect://127.0.0.1:5600'
```

Then:

```sh
sudo systemctl enable --now rtsp@h265
```

On the PC:

```text
rtsp://ORANGE_PI_IP:8554/wfb
```

Example:

```sh
gst-launch-1.0 rtspsrc latency=0 location=rtsp://192.168.50.2:8554/wfb ! rtph264depay ! avdec_h264 ! videoconvert ! autovideosink sync=false
```

`wfb_rtsp` listens for local RTP on UDP `5600`, repackages it, and serves RTSP on `8554`.

#### Option 2: UDP multicast

Use administratively scoped multicast, for example `239.x.x.x`:

```ini
[gs_video]
peer = 'connect://239.50.50.50:5600'
```

On the PC:

```sh
gst-launch-1.0 -v udpsrc multicast-group=239.50.50.50 port=5600 auto-multicast=true caps='application/x-rtp, media=(string)video, clock-rate=(int)90000,encoding-name=(string)H265' ! rtph265depay ! avdec_h265 ! videoconvert ! autovideosink sync=false
```

If multicast does not leave the Orange Pi on the right interface, add a route, replacing `eth0` with your LAN interface:

```sh
sudo ip route add 239.0.0.0/8 dev eth0
```
Check both machines:
```sh
sudo tcpdump -ni eth0 host 239.50.50.50 and udp port 5600
```

### What systemd runs
-----------------

`wifibroadcast@gs` runs:

```sh
/usr/bin/python3 /usr/bin/wfb-server --profiles gs --wlans wlan0
```

`wfb-server` reads `/etc/wifibroadcast.cfg`, puts `wlan0` into monitor mode,
sets the channel, then starts per-stream helpers.

### Deb package installs
--------------------

- `/usr/bin`: `wfb_tx`, `wfb_rx`, `wfb_keygen`, `wfb_tx_cmd`, `wfb_rtsp`,
  `wfb-cli`, `wfb-server`, `wfb-log-parser`, `wfb-test-latency`,
  `wfb-cli-x11`, `wfb-nics`, bind helper scripts.
- `/lib/systemd/system`: `wifibroadcast.service`, `wifibroadcast@.service`,
  `wfb-cluster*.service`, `rtsp@.service`.
- `/etc/default`: `wifibroadcast`, `wifibroadcast.drone_bind`,
  `wifibroadcast.gs_bind`.
- `/etc/sysctl.d/98-wifibroadcast.conf`.
- `/etc/logrotate.d/wifibroadcast`.
- Python package `wfb_ng` and bundled config defaults `master.cfg/site.cfg`.

### Troubleshooting
---------------

Confirm adapter driver:

```sh
ethtool -i wlan0
```

If needed:

```sh
sudo tee /etc/modprobe.d/wfb-rtl8812au.conf >/dev/null <<'EOF'
blacklist rtw88_8812au
blacklist rtw88_usb
blacklist rtw88_core
blacklist 8812au
blacklist 88XXau
EOF
```

Manual RX test:

```sh
sudo systemctl stop wifibroadcast@gs
sudo /usr/bin/wfb_rx -p 0 -c 127.0.0.1 -u 5600 -K /etc/gs.key -R 2097152 -s 2097152 -l 1000 -i wlan0
```

#### To check WIFI device able to work with WFB

Raw monitor packet check:

```sh
sudo systemctl stop wifibroadcast@gs
sudo ip link set wlan0 down
sudo iw dev wlan0 set type monitor
sudo ip link set wlan0 up
sudo iw dev wlan0 set channel 161 HT20
sudo tcpdump -ni wlan0 -e -s 256
```

If Orange shows packets and Radxa shows nothing with the same dongle, location, and channel, suspect Radxa USB/power/driver/platform issue.

### Driver compile (if fails)
--------------

Patch the top of `core/rtw_br_ext.c` like this:

```c
#ifdef __KERNEL__
      #include <linux/version.h>
      #include <linux/if_arp.h>
      #include <net/ip.h>
      #include <linux/atalk.h>
      #include <linux/udp.h>
      #include <linux/if_pppox.h>
#endif
```

The added line is:

```c
#include <linux/version.h>
```

Then on Radxa:

```sh
cd /path/to/rtl8812au
make clean
make -j$(nproc) KVER=$(uname -r) KSRC=/lib/modules/$(uname -r)/build
sudo make install
sudo depmod -a
```

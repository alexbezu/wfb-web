# WFB-ng tunnel notes

Tunnel is enabled by default in the stock `gs` and `drone` profiles.

Default GS streams include:

```ini
[gs]
streams = [{'name': 'video',   'stream_rx': 0x00, 'stream_tx': None, 'service_type': 'udp_direct_rx',  'profiles': ['base', 'gs_base', 'video', 'gs_video']},
           {'name': 'mavlink', 'stream_rx': 0x10, 'stream_tx': 0x90, 'service_type': 'mavlink', 'profiles': ['base', 'gs_base', 'mavlink', 'gs_mavlink']},
           {'name': 'tunnel',  'stream_rx': 0x20, 'stream_tx': 0xa0, 'service_type': 'tunnel',  'profiles': ['base', 'gs_base', 'tunnel', 'gs_tunnel']}]
```

So these processes are tunnel-related:

```sh
/usr/bin/wfb_rx -p 32 -U tunnel-rx-8fb5535b ...
/usr/bin/wfb_tx -f data -p 160 -U tunnel-tx-4924b565 ...
```

Meaning:

- `-p 32` / `0x20`: GS tunnel RX stream.
- `-p 160` / `0xa0`: GS tunnel TX stream.
- `tunnel-rx-*` and `tunnel-tx-*`: abstract Unix sockets used internally by `wfb-server` and the tunnel/TUN-TAP handler.

This does not disable tunnel:

```ini
[gs_tunnel]
default_route = False
```

It only prevents the tunnel interface from becoming the default route. The tunnel service still starts.

To disable tunnel, remove the tunnel stream from the top-level profile. For simple one-way video RX on GS:

```ini
[gs]
streams = [{'name': 'video', 'stream_rx': 0x00, 'stream_tx': None, 'service_type': 'udp_direct_rx', 'profiles': ['base', 'gs_base', 'video', 'gs_video']}]
stats_port = 8003
api_port = 8103
link_domain = "default"
```

For simple video TX on drone:

```ini
[drone]
streams = [{'name': 'video', 'stream_rx': None, 'stream_tx': 0x00, 'service_type': 'udp_direct_tx', 'profiles': ['base', 'drone_base', 'video', 'drone_video']}]
stats_port = 8002
api_port = 8102
link_domain = "default"
```

Restart:

```sh
sudo systemctl restart wifibroadcast@gs
sudo systemctl restart wifibroadcast@drone
```

Check:

```sh
systemctl status wifibroadcast@gs
ps aux | grep -E 'wfb_rx|wfb_tx'
```

After disabling tunnel, there should be no `tunnel-rx-*` or `tunnel-tx-*` processes.

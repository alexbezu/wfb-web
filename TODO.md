Service - systemctl (must have rights to alter `/etc/wifibroadcast.cfg`) or any suggestion

  - Backend: Go HTTP service, similar in shape to irizi-web, with embedded Vite frontend like /Users/oleksii/irizi-web/internal/frontend/frontend.go:15.
  - Frontend: small TypeScript/Vite UI for config editing, service control, and live stats.
  - Config support:
      - edit /etc/wifibroadcast.cfg fields from /Users/oleksii/OpenIPC/wfb-ng/gs_video_notes.md:7: [common], [base], [gs_video]
      - edit /etc/default/wifibroadcast for WFB_NICS, RTP_MTU, RTP_JITTER, RTSP_PORT, RTSP_URI

  - Stats/API:
      - connect to WFB-NG GS JSON API on port 8103, defined in /Users/oleksii/OpenIPC/wfb-ng/wfb_ng/conf/master.cfg:154
      - consume line-delimited JSON settings/RX/TX stats from /Users/oleksii/OpenIPC/wfb-ng/wfb_ng/protocols.py:72

  - Service control:
      - systemctl start/stop/restart/status wifibroadcast@gs
      - optionally rtsp@h265 rtsp@h264
      - safest deployment is a systemd service running as root or with tightly scoped sudo rules.


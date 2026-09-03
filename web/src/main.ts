import "./style.css";

type Config = {
  common: { wifi_channel: number; wifi_region: string; link_domain: string };
  base: { ldpc: number; stbc: number; bandwidth: number; mcs_index: number; force_vht: boolean };
  gs_video: { peer: string };
  default: {
    profile: string;
    auto_services: boolean;
    wfb_nics: string; rtp_mtu: number; rtp_jitter: number; rtsp_port: number; rtsp_uri: string; rtsp_codec: string;
    camera_enabled: boolean; camera_source: string; camera_device: string; camera_rtsp_url: string; camera_codec: string; camera_host: string; camera_port: number;
    camera_width: number; camera_height: number; camera_framerate: number; camera_bitrate: number; camera_mtu: number; camera_test_pattern: string;
  };
};

type EffectiveConfig = {
  files: { master: string; local: string; default: string };
  sections: Array<{ name: string; fields: EffectiveField[] }>;
};

type EffectiveField = {
  section: string;
  key: string;
  value: string;
  default_value: string;
  default: boolean;
  changed: boolean;
  source: string;
  comment: string;
  editable: boolean;
};

type ServiceState = {
  unit: string;
  active: string;
  sub: string;
  load: string;
  unit_file: string;
  can_reload: boolean;
};

type RTSPState = {
  unit: string;
  active: string;
  sub: string;
  options: { codec: string; mtu: number; port: number; uri: string; latency: number; rtp_port: number };
  url: string;
  native: boolean;
  error?: string;
};

type CameraState = {
  unit: string;
  active: string;
  sub: string;
  options: {
    enabled: boolean; source: string; device: string; rtsp_url: string; codec: string; host: string; port: number; width: number; height: number;
    framerate: number; bitrate: number; mtu: number; pattern: string;
  };
  output: string;
  command: string;
  error?: string;
};

type ProfileSelection = {
  profile: string;
  source: string;
  options: Array<{ profile: string; label: string; api_addr: string }>;
};

type RadioInfo = {
  name: string;
  ethtool: string;
  iw: string;
  error?: string;
};

type KeyInfo = {
  profile: string;
  path: string;
  exists: boolean;
  size: number;
  hash: string;
  mod_time: string;
};

type WFBSettingsEvent = {
  type: "settings";
  profile?: string;
  is_cluster?: boolean;
  wlans?: string[];
};

type WFBRXAntennaStats = {
  ant?: number;
  freq?: number;
  mcs?: number;
  bw?: number;
  pkt_recv?: number;
  rssi_min?: number;
  rssi_avg?: number;
  rssi_max?: number;
  snr_min?: number;
  snr_avg?: number;
  snr_max?: number;
};

type WFBTXAntennaStats = {
  ant?: number;
  pkt_sent?: number;
  pkt_drop?: number;
  lat_min?: number;
  lat_avg?: number;
  lat_max?: number;
};

type WFBStatsEvent = {
  type: "rx" | "tx" | string;
  timestamp?: number;
  id?: number | string;
  tx_wlan?: number | null;
  packets?: Record<string, unknown>;
  session?: Record<string, unknown>;
  rx_ant_stats?: WFBRXAntennaStats[];
  tx_ant_stats?: WFBTXAntennaStats[];
  rf_temperature?: Record<string, number>;
};

type VideoHealthSample = {
  time: number;
  snrAvg: number | null;
  rxPackets: number | null;
  dropRate: number | null;
  latencyAvg: number | null;
  txWlan: number | null;
};

const root = document.querySelector<HTMLDivElement>("#app");
if (!root) {
  throw new Error("missing app element");
}
const app = root;

let config: Config | null = null;
let effectiveConfig: EffectiveConfig | null = null;
let profileSelection: ProfileSelection | null = null;
let services: ServiceState[] = [];
let rtspState: RTSPState | null = null;
let cameraState: CameraState | null = null;
let radios: RadioInfo[] = [];
let keyInfo: KeyInfo | null = null;
let settingsEvent: WFBSettingsEvent | null = null;
let rxEvent: WFBStatsEvent | null = null;
let txEvent: WFBStatsEvent | null = null;
let statsConnected = false;
let error = "";
let activeTab = "stats";
let statsSource: EventSource | null = null;
let configSearch = "";
let configSection = "all";
let configChangedOnly = false;
let configDrafts = new Map<string, string>();
let copiedEndpoint = "";
let multicastIface = "eth0";
let videoHealthHistory: VideoHealthSample[] = [];
let previousTxTotals: { sent: number; drop: number } | null = null;
const maxVideoHealthSamples = 90;

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error ?? response.statusText);
  }
  return body as T;
}

async function load(): Promise<void> {
  try {
    config = await requestJSON<Config>("/api/config");
    effectiveConfig = await requestJSON<EffectiveConfig>("/api/config/effective");
    profileSelection = await requestJSON<ProfileSelection>("/api/profile");
    services = await requestJSON<ServiceState[]>("/api/services");
    rtspState = await requestJSON<RTSPState>("/api/rtsp");
    cameraState = await requestJSON<CameraState>("/api/camera");
    radios = await requestJSON<RadioInfo[]>("/api/radio");
    keyInfo = await requestJSON<KeyInfo>("/api/key");
    error = "";
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
  }
  render();
}

async function selectProfile(profile: string): Promise<void> {
  try {
    profileSelection = await requestJSON<ProfileSelection>("/api/profile", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ profile })
    });
    settingsEvent = null;
    rxEvent = null;
    txEvent = null;
    keyInfo = await requestJSON<KeyInfo>("/api/key");
    statsConnected = false;
    streamStats();
    render();
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    render();
  }
}

async function uploadKey(file: File): Promise<void> {
  try {
    keyInfo = await requestJSON<KeyInfo>("/api/key", {
      method: "PUT",
      headers: { "Content-Type": "application/octet-stream" },
      body: await file.arrayBuffer()
    });
    error = "";
    render();
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    render();
  }
}

async function saveConfig(): Promise<void> {
  if (!config || configDrafts.size === 0) {
    return;
  }
  try {
    const updates = Array.from(configDrafts.entries()).map(([id, value]) => {
      const [section, key] = id.split(".", 2);
      return { section, key, value };
    });
    effectiveConfig = await requestJSON<EffectiveConfig>("/api/config/params", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ updates })
    });
    configDrafts = new Map();
    await load();
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    render();
  }
}

async function serviceAction(unit: string, action: string): Promise<void> {
  try {
    await requestJSON(`/api/services/${unit}/${action}`, { method: "POST" });
    await load();
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    render();
  }
}

function render(): void {
  app.replaceChildren(
    el("main", { class: "shell" },
      el("aside", { class: "side" },
        el("h1", {}, "WFB Web"),
        el("p", {}, "Ground station sidecar"),
        renderProfileSelect(),
        renderNav()
      ),
      el("section", { class: "content" },
        error ? el("div", { class: "error" }, error) : "",
        renderActiveTab()
      )
    )
  );
}

function renderProfileSelect(): HTMLElement {
  const selection = profileSelection;
  if (!selection) {
    return el("div", { class: "profile-box" }, el("span", {}, "Profile"), el("strong", {}, "-"));
  }
  return el("label", { class: "profile-box" },
    "Profile",
    el("select", {
      value: selection.profile,
      onChange: (event: Event) => selectProfile((event.target as HTMLSelectElement).value)
    }, ...selection.options.map((option) =>
      el("option", {
        value: option.profile,
        selected: String(option.profile === selection.profile)
      }, `${option.label} (${option.profile})`)
    )),
    el("small", {}, `source: ${selection.source}`)
  );
}

function renderNav(): HTMLElement {
  const tabs = [
    ["stats", "Live Stats"],
    ["endpoints", "Endpoints"],
    ["config", "Configuration"],
    ["key", "Key"],
    ["radio", "Radio"],
    ["services", "Services"]
  ];
  return el("nav", { class: "nav" }, ...tabs.map(([id, label]) =>
    el("button", {
      class: activeTab === id ? "active" : "",
      onClick: () => {
        activeTab = id;
        void load();
      }
    }, label)
  ));
}

function renderActiveTab(): HTMLElement {
  switch (activeTab) {
    case "endpoints":
      return renderEndpoints();
    case "config":
      return renderConfig();
    case "key":
      return renderKey();
    case "radio":
      return renderRadio();
    case "services":
      return renderServices();
    default:
      return renderStats();
  }
}

function renderEndpoints(): HTMLElement {
  if (!config) {
    return el("div", { class: "panel" }, "No config loaded");
  }

  const peer = parseConnectPeer(config.gs_video.peer);
  const rtspOptions = rtspState?.options;
  const codec = rtspOptions?.codec === "h264" ? "h264" : "h265";
  const gstMode = codec === "h264" ? "H264" : "H265";
  const depay = codec === "h264" ? "rtph264depay" : "rtph265depay";
  const rtspPort = rtspOptions?.port ?? config.default.rtsp_port;
  const rtspURI = rtspOptions?.uri ?? config.default.rtsp_uri;
  const rtspURL = `rtsp://${window.location.hostname || "127.0.0.1"}:${rtspPort}${rtspURI}`;
  const endpoints = [];
  const iface = multicastIface.trim() || "eth0";
  const camera = cameraState?.options;
  if (camera) {
    const cameraMode = camera.codec === "h265" ? "H265" : "H264";
    const cameraDepay = camera.codec === "h265" ? "rtph265depay" : "rtph264depay";
    const cameraCaps = `application/x-rtp,media=video,clock-rate=90000,encoding-name=${cameraMode}`;
    endpoints.push({
      title: "Camera RTP",
      value: `udp://${camera.host}:${camera.port}`,
      detail: `wfb-web-camera: ${cameraState?.active ?? "unknown"}/${cameraState?.sub ?? "unknown"}, source ${camera.source}`,
      commands: [
        `gst-launch-1.0 -v udpsrc address=${camera.host} port=${camera.port} caps='${cameraCaps}' ! ${cameraDepay} ! decodebin ! autovideosink sync=false`,
        `CODEC=${camera.codec} HOST=${camera.host} PORT=${camera.port} ./scripts/watch-camera-video`
      ]
    });
  }

  if (peer && peer.addr === "127.0.0.1") {
    endpoints.push({
      title: "RTSP",
      value: rtspURL,
      detail: nativeRTSPState(),
      commands: [
        `gst-launch-1.0 rtspsrc latency=0 location=${rtspURL} ! decodebin ! autovideosink sync=false`,
        `vlc ${rtspURL}`
      ]
    });
  }

  if (peer) {
    const isMulticast = isMulticastAddress(peer.addr);
    const caps = `application/x-rtp,media=video,clock-rate=90000,encoding-name=${gstMode}`;
    endpoints.push({
      title: isMulticast ? "UDP Multicast" : "UDP Unicast",
      value: `udp://${peer.addr}:${peer.port}`,
      detail: config.gs_video.peer,
      commands: [
        isMulticast
          ? `gst-launch-1.0 -v udpsrc multicast-group=${peer.addr} multicast-iface=${iface} port=${peer.port} auto-multicast=true caps='${caps}' ! ${depay} ! decodebin ! autovideosink sync=false`
          : `gst-launch-1.0 -v udpsrc address=${peer.addr} port=${peer.port} caps='${caps}' ! ${depay} ! decodebin ! autovideosink sync=false`,
        `vlc udp://@${isMulticast ? peer.addr : ""}:${peer.port}`
      ]
    });
  }

  return el("div", { class: "panel" },
    el("div", { class: "panel-head" },
      el("div", {},
        el("h2", {}, "Endpoints"),
        el("p", { class: "muted" }, `GS video peer: ${config.gs_video.peer}`)
      ),
      el("div", { class: "actions" },
        el("label", { class: "compact-field" },
          "Multicast iface",
          el("input", {
            value: multicastIface,
            placeholder: "eth0",
            onInput: (event: Event) => {
              multicastIface = (event.target as HTMLInputElement).value;
              render();
            }
          })
        ),
        el("button", { class: "secondary", onClick: () => void load() }, "Refresh")
      )
    ),
    endpoints.length
      ? el("div", { class: "endpoint-grid" }, ...endpoints.map(renderEndpoint))
      : el("p", { class: "muted" }, "No connect:// video endpoint configured")
  );
}

function renderEndpoint(endpoint: { title: string; value: string; detail: string; commands: string[] }): HTMLElement {
  return el("section", { class: "endpoint-card" },
    el("div", { class: "endpoint-head" },
      el("div", {},
        el("h3", {}, endpoint.title),
        el("code", {}, endpoint.value),
        el("small", {}, endpoint.detail)
      ),
      el("button", { class: "compact", onClick: () => void copyEndpoint(endpoint.value) }, copiedEndpoint === endpoint.value ? "Copied" : "Copy")
    ),
    el("div", { class: "endpoint-commands" },
      ...endpoint.commands.map((command) =>
        el("div", { class: "command-row" },
          el("code", {}, command),
          el("button", { class: "secondary compact", onClick: () => void copyEndpoint(command) }, copiedEndpoint === command ? "Copied" : "Copy")
        )
      )
    )
  );
}

async function copyEndpoint(value: string): Promise<void> {
  try {
    await writeClipboard(value);
    copiedEndpoint = value;
    render();
    window.setTimeout(() => {
      if (copiedEndpoint === value) {
        copiedEndpoint = "";
        render();
      }
    }, 1400);
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    render();
  }
}

async function writeClipboard(value: string): Promise<void> {
  if (navigator.clipboard) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.append(textarea);
  textarea.focus();
  textarea.select();
  const ok = document.execCommand("copy");
  textarea.remove();
  if (!ok) {
    throw new Error("copy failed");
  }
}

function parseConnectPeer(peer: string): { addr: string; port: number } | null {
  const match = /^connect:\/\/([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+):([0-9]+)$/i.exec(peer.trim());
  if (!match) {
    return null;
  }
  return { addr: match[1], port: Number(match[2]) };
}

function isMulticastAddress(addr: string): boolean {
  const first = Number(addr.split(".", 1)[0]);
  return first >= 224 && first <= 239;
}

function nativeRTSPState(): string {
  if (!rtspState) {
    return "native RTSP: unknown";
  }
  const inputPort = rtspState.options.rtp_port || 5600;
  const native = rtspState.native ? "native" : "not built";
  return `${native} RTSP: ${rtspState.active}/${rtspState.sub}, input udp://127.0.0.1:${inputPort}`;
}

function renderServices(): HTMLElement {
  return el("div", { class: "panel" },
    el("h2", {}, "Services"),
    services.length
      ? el("div", { class: "table-wrap" },
        el("table", { class: "service-table" },
          el("thead", {}, el("tr", {},
            el("th", {}, "Service"),
            el("th", {}, "Actions"),
            el("th", {}, "State"),
            el("th", {}, "Load")
          )),
          el("tbody", {}, ...services.map(renderServiceRow))
        )
      )
      : el("p", { class: "muted" }, "No service data")
  );
}

function renderServiceRow(service: ServiceState): HTMLElement {
  const key = serviceKey(service.unit);
  const disabledReason = serviceDisabledReason(key);
  const disabled = disabledReason !== "";
  return el("tr", {},
    el("td", {},
      el("span", { class: `status-dot ${service.active === "active" ? "active" : ""}` }),
      service.unit
    ),
    el("td", {},
      el("div", { class: "row-actions" },
        el("button", { class: "secondary compact", disabled: String(disabled), title: disabledReason, onClick: () => serviceAction(key, "start") }, "Start"),
        el("button", { class: "secondary compact", disabled: String(disabled), title: disabledReason, onClick: () => serviceAction(key, "stop") }, "Stop"),
        el("button", { class: "compact", disabled: String(disabled), title: disabledReason, onClick: () => serviceAction(key, "restart") }, "Restart"),
        supportsEnableDisable(key)
          ? el("button", { class: "secondary compact", disabled: String(disabled), title: disabledReason, onClick: () => serviceAction(key, "enable") }, "Enable")
          : "",
        supportsEnableDisable(key)
          ? el("button", { class: "secondary compact", disabled: String(disabled), title: disabledReason, onClick: () => serviceAction(key, "disable") }, "Disable")
          : ""
      ),
      disabledReason ? el("small", { class: "muted" }, disabledReason) : ""
    ),
    el("td", {}, `${service.active}/${service.sub}`),
    el("td", {}, service.load || "-")
  );
}

function serviceDisabledReason(key: string): string {
  const selected = profileSelection?.profile ?? "gs";
  const gsOnly = new Set(["wifibroadcast-gs", "rtsp-h265", "rtsp-h264", "wfb-web-rtsp"]);
  const droneOnly = new Set(["wifibroadcast-drone", "fpv-camera", "wfb-web-camera"]);
  if (selected === "drone" && gsOnly.has(key)) {
    return "Ground-station service disabled for drone profile";
  }
  if (selected === "gs" && droneOnly.has(key)) {
    return "Drone service disabled for ground-station profile";
  }
  return "";
}

function supportsEnableDisable(key: string): boolean {
  return key !== "wfb-web-rtsp" && key !== "wfb-web-camera";
}

function serviceKey(unit: string): string {
  switch (unit) {
    case "wifibroadcast@gs":
    case "wifibroadcast@gs.service":
      return "wifibroadcast-gs";
    case "wifibroadcast@drone":
    case "wifibroadcast@drone.service":
      return "wifibroadcast-drone";
    case "rtsp@h265":
    case "rtsp@h265.service":
      return "rtsp-h265";
    case "rtsp@h264":
    case "rtsp@h264.service":
      return "rtsp-h264";
    case "wfb-web-rtsp":
      return "wfb-web-rtsp";
    case "wfb-web-camera":
      return "wfb-web-camera";
    default:
      return "fpv-camera";
  }
}

function renderConfig(): HTMLElement {
  if (!config || !effectiveConfig) {
    return el("div", { class: "panel" }, "No config loaded");
  }
  const fields = allConfigFields();
  const changed = fields.filter((fieldInfo) => isFieldChanged(fieldInfo)).length;
  return el("div", { class: "panel" },
    el("div", { class: "panel-head" },
      el("div", {},
        el("h2", {}, "Configuration"),
        el("p", { class: "muted" }, `${fields.length} parameters, ${changed} non-default, ${configDrafts.size} pending edit${configDrafts.size === 1 ? "" : "s"}`)
      ),
      el("div", { class: "actions" },
        el("button", { disabled: String(configDrafts.size === 0), onClick: () => saveConfig() }, "Save"),
        el("button", { class: "secondary", disabled: String(configDrafts.size === 0), onClick: () => { configDrafts = new Map(); render(); } }, "Discard")
      )
    ),
    el("div", { class: "file-grid" },
      fileBadge("master.cfg", effectiveConfig.files.master || "not found"),
      fileBadge("wifibroadcast.cfg", effectiveConfig.files.local),
      fileBadge("default", effectiveConfig.files.default)
    ),
    renderStandardConfig(),
    renderConfigToolbar(),
    renderParameterConfigTable()
  );
}

function renderStandardConfig(): HTMLElement {
  const selectedProfile = profileSelection?.profile ?? "gs";
  const standard = [
    ["default", "WFB_WEB_PROFILE", "Saved Profile"],
    ["common", "wifi_channel", "WiFi Channel"],
    ["common", "wifi_region", "WiFi Region"],
    ["common", "wifi_txpower", "TX Power"],
    ["common", "link_domain", "Link Domain"],
    ["base", "bandwidth", "Bandwidth"],
    ["base", "mcs_index", "MCS Index"],
    ["base", "ldpc", "LDPC"],
    ["base", "stbc", "STBC"],
    ["base", "force_vht", "Force VHT"],
    ["default", "WFB_NICS", "WFB NICS"]
  ];
  if (selectedProfile === "gs") {
    standard.push(
      ["gs_video", "peer", "GS Video Peer"],
      ["default", "WFB_WEB_RTSP_CODEC", "GS RTSP Codec"]
    );
  }
  if (selectedProfile === "drone") {
    standard.push(
    ["default", "WFB_WEB_CAMERA_ENABLED", "Camera Enabled"],
    ["default", "WFB_WEB_CAMERA_SOURCE", "Camera Source"],
    ["default", "WFB_WEB_CAMERA_RTSP_URL", "Camera RTSP URL"],
    ["default", "WFB_WEB_CAMERA_CODEC", "Camera Codec"]
    );
  }
  return el("section", { class: "config-section" },
    el("h3", {}, "Standard"),
    el("div", { class: "standard-grid" }, ...standard.map(([section, key, label]) => {
      const fieldInfo = findConfigField(section, key);
      return renderParamControl(label, fieldInfo);
    }))
  );
}

function renderParamControl(label: string, fieldInfo: EffectiveField | null): HTMLElement {
  if (!fieldInfo) {
    return el("label", {}, label, el("input", { disabled: "true", value: "-" }));
  }
  if (isBooleanParam(fieldInfo)) {
    return renderBooleanParamControl(label, fieldInfo);
  }
  const id = fieldID(fieldInfo);
  const valueNow = configDrafts.get(id) ?? fieldInfo.value;
  const attrs: Record<string, string | ((event: Event) => void)> = {
    value: valueNow,
    onChange: (event: Event) => { setConfigDraft(fieldInfo, (event.target as HTMLInputElement).value); render(); }
  };
  return el("label", { class: isFieldChanged(fieldInfo) ? "changed" : "" },
    label,
    el("input", attrs),
    el("small", {}, fieldInfo.comment || `default: ${fieldInfo.default_value || "-"}`)
  );
}

function renderBooleanParamControl(label: string, fieldInfo: EffectiveField): HTMLElement {
  const id = fieldID(fieldInfo);
  const valueNow = configDrafts.get(id) ?? fieldInfo.value;
  return el("label", { class: isFieldChanged(fieldInfo) ? "changed inline-toggle" : "inline-toggle" },
    el("input", {
      type: "checkbox",
      checked: String(parseBool(valueNow)),
      onChange: (event: Event) => {
        setConfigDraft(fieldInfo, formatBooleanParam(fieldInfo, (event.target as HTMLInputElement).checked));
        render();
      }
    }),
    label,
    el("small", {}, fieldInfo.comment || `default: ${fieldInfo.default_value || "-"}`)
  );
}

function renderConfigToolbar(): HTMLElement {
  const sections = ["all", ...new Set(effectiveConfig?.sections.map((section) => section.name) ?? [])];
  return el("div", { class: "config-toolbar" },
    el("input", {
      value: configSearch,
      placeholder: "Search parameters",
      onInput: (event: Event) => { configSearch = (event.target as HTMLInputElement).value; render(); }
    }),
    el("select", {
      value: configSection,
      onChange: (event: Event) => { configSection = (event.target as HTMLSelectElement).value; render(); }
    }, ...sections.map((section) => el("option", { value: section, selected: String(section === configSection) }, section === "all" ? "All sections" : `[${section}]`))),
    el("label", { class: "inline-toggle" },
      el("input", {
        type: "checkbox",
        checked: String(configChangedOnly),
        onChange: (event: Event) => { configChangedOnly = (event.target as HTMLInputElement).checked; render(); }
      }),
      "Changed only"
    )
  );
}

function renderParameterConfigTable(): HTMLElement {
  const fields = filteredConfigFields();
  if (fields.length === 0) {
    return el("p", { class: "muted" }, "No matching parameters");
  }
  return el("div", { class: "param-table-wrap" },
    el("table", { class: "param-table" },
      el("thead", {}, el("tr", {},
        el("th", {}, "Status"),
        el("th", {}, "Parameter"),
        el("th", {}, "Value"),
        el("th", {}, "Default"),
        el("th", {}, "Source"),
        el("th", {}, "")
      )),
      el("tbody", {}, ...fields.map(renderConfigRow))
    )
  );
}

function renderConfigRow(fieldInfo: EffectiveField): HTMLElement {
  const id = fieldID(fieldInfo);
  const valueNow = configDrafts.get(id) ?? fieldInfo.value;
  return el("tr", { class: isFieldChanged(fieldInfo) ? "row-changed" : "" },
    el("td", {}, statusPill(configDrafts.has(id) ? "pending" : (isFieldChanged(fieldInfo) ? "changed" : "default"))),
    el("td", {},
      el("strong", {}, `${fieldInfo.section}.${fieldInfo.key}`),
      fieldInfo.comment ? el("small", {}, fieldInfo.comment) : ""
    ),
    el("td", {}, el("textarea", {
      value: valueNow,
      rows: String(Math.min(5, Math.max(1, valueNow.split("\n").length))),
      onChange: (event: Event) => { setConfigDraft(fieldInfo, (event.target as HTMLTextAreaElement).value); render(); }
    })),
    el("td", {}, el("code", {}, fieldInfo.default_value || "-")),
    el("td", {}, fieldInfo.source),
    el("td", {}, el("button", {
      class: "icon-button",
      disabled: String(!fieldInfo.default_value && !configDrafts.has(id)),
      title: "Reset to default",
      onClick: () => {
        if (fieldInfo.default_value) {
          setConfigDraft(fieldInfo, fieldInfo.default_value);
        } else {
          configDrafts.delete(id);
        }
        render();
      }
    }, "Reset"))
  );
}

function statusPill(label: string): HTMLElement {
  return el("span", { class: `pill ${label}` }, label);
}

function allConfigFields(): EffectiveField[] {
  return effectiveConfig?.sections.flatMap((section) => section.fields) ?? [];
}

function filteredConfigFields(): EffectiveField[] {
  const needle = configSearch.trim().toLowerCase();
  return allConfigFields().filter((fieldInfo) => {
    if (configSection !== "all" && fieldInfo.section !== configSection) {
      return false;
    }
    if (configChangedOnly && !isFieldChanged(fieldInfo)) {
      return false;
    }
    if (!needle) {
      return true;
    }
    return `${fieldInfo.section}.${fieldInfo.key} ${fieldInfo.value} ${fieldInfo.comment}`.toLowerCase().includes(needle);
  });
}

function findConfigField(section: string, key: string): EffectiveField | null {
  return allConfigFields().find((fieldInfo) => fieldInfo.section === section && fieldInfo.key === key) ?? null;
}

function fieldID(fieldInfo: EffectiveField): string {
  return `${fieldInfo.section}.${fieldInfo.key}`;
}

function setConfigDraft(fieldInfo: EffectiveField, valueNow: string): void {
  const id = fieldID(fieldInfo);
  if (valueNow === fieldInfo.value) {
    configDrafts.delete(id);
  } else {
    configDrafts.set(id, valueNow);
  }
}

function isFieldChanged(fieldInfo: EffectiveField): boolean {
  const valueNow = configDrafts.get(fieldID(fieldInfo)) ?? fieldInfo.value;
  return Boolean(fieldInfo.default_value) && normalizeParamValue(valueNow) !== normalizeParamValue(fieldInfo.default_value);
}

function isBooleanParam(fieldInfo: EffectiveField): boolean {
  const valueNow = (configDrafts.get(fieldID(fieldInfo)) ?? fieldInfo.value).toLowerCase();
  const defaultValue = fieldInfo.default_value.toLowerCase();
  return ["true", "false"].includes(valueNow) || ["true", "false"].includes(defaultValue);
}

function formatBooleanParam(fieldInfo: EffectiveField, checked: boolean): string {
  if (fieldInfo.section !== "default") {
    return checked ? "True" : "False";
  }
  return checked ? "true" : "false";
}

function parseBool(valueNow: string): boolean {
  return valueNow.trim().toLowerCase() === "true";
}

function normalizeParamValue(valueNow: string): string {
  return valueNow.split(/\s+/).filter(Boolean).join(" ");
}

function fileBadge(label: string, path: string): HTMLElement {
  return el("div", { class: "file-badge" }, el("span", {}, label), el("strong", {}, path));
}

function renderKey(): HTMLElement {
  const info = keyInfo;
  const input = el("input", {
    type: "file",
    onChange: (event: Event) => {
      const file = (event.target as HTMLInputElement).files?.[0];
      if (file) {
        void uploadKey(file);
      }
    }
  }) as HTMLInputElement;

  return el("div", { class: "panel" },
    el("h2", {}, "Key"),
    el("div", { class: "summary-grid" },
      statCard("Profile", profileSelection?.profile ?? "-", `source: ${profileSelection?.source ?? "-"}`),
      statCard("Path", info?.path ?? "-", info?.exists ? "present" : "missing"),
      statCard("Hash", info?.hash || "-", "sha256 short"),
      statCard("Size", info?.exists ? `${info.size} bytes` : "-", info?.mod_time || "-")
    ),
    el("div", { class: "actions" },
      el("button", {
        onClick: () => input.click()
      }, "Upload key"),
      input
    )
  );
}

function renderRadio(): HTMLElement {
  return el("div", { class: "panel" },
    el("h2", {}, "Radio"),
    radios.length
      ? el("div", { class: "radio-grid" }, ...radios.map(renderRadioCard))
      : el("p", { class: "muted" }, "No radio interfaces configured")
  );
}

function renderRadioCard(info: RadioInfo): HTMLElement {
  return el("section", { class: "radio-card" },
    el("h3", {}, info.name),
    info.error ? el("p", { class: "error" }, info.error) : "",
    el("h4", {}, "ethtool -i"),
    el("pre", {}, info.ethtool || "-"),
    el("h4", {}, "iw dev info"),
    el("pre", {}, info.iw || "-")
  );
}

function renderStats(): HTMLElement {
  return el("div", { class: "panel" },
    el("h2", {}, "Live Stats"),
    el("div", { class: "summary-grid" },
      statCard("Stream", statsConnected ? "connected" : "waiting", profileSelection?.options.find((option) => option.profile === profileSelection?.profile)?.api_addr ?? "-"),
      statCard("Profile", settingsEvent?.profile ?? profileSelection?.profile ?? "-", settingsEvent?.is_cluster ? "cluster" : `source: ${profileSelection?.source ?? "-"}`),
      statCard("WLANs", settingsEvent?.wlans?.join(", ") || config?.default.wfb_nics || "-", "configured radios"),
      statCard("TX WLAN", value(rxEvent?.tx_wlan), "selected antenna")
    ),
    renderVideoHealth(),
    el("div", { class: "stats-layout" },
      renderRXStats(),
      renderTXStats()
    ),
    renderPacketStats()
  );
}

function statCard(label: string, main: string, hint: string): HTMLElement {
  return el("div", { class: "metric" }, el("span", {}, label), el("strong", {}, main), el("small", {}, hint));
}

function renderVideoHealth(): HTMLElement {
  const latest = videoHealthHistory.at(-1) ?? currentVideoHealthSample(Date.now(), false);
  return el("section", { class: "stat-section video-health" },
    el("div", { class: "section-head" },
      el("h3", {}, "Video Health"),
      el("small", { class: "muted" }, `${videoHealthHistory.length} recent samples`)
    ),
    el("div", { class: "health-grid" },
      healthCard("Packet loss", percent(latest.dropRate), lossHint(latest.dropRate), latest.dropRate, renderSparkline("dropRate", { min: 0, max: Math.max(10, maxHistory("dropRate") ?? 10), invert: true, suffix: "%" })),
      healthCard("SNR", formatNumber(latest.snrAvg, " dB"), thresholdHint(latest.snrAvg, 12, 20, "higher is better"), latest.snrAvg, renderSparkline("snrAvg", { min: 0, max: Math.max(35, maxHistory("snrAvg") ?? 35) })),
      healthCard("Latency", formatNumber(latest.latencyAvg, " ms"), thresholdHint(latest.latencyAvg, 25, 10, "lower is better", true), latest.latencyAvg, renderSparkline("latencyAvg", { min: 0, max: Math.max(40, maxHistory("latencyAvg") ?? 40), invert: true, suffix: "ms" })),
      healthCard("RX packets", formatNumber(latest.rxPackets, " pkt/s"), "watch for sudden dips", latest.rxPackets, renderSparkline("rxPackets", { min: 0, max: Math.max(1, maxHistory("rxPackets") ?? 1) })),
      healthCard("TX WLAN", value(latest.txWlan), "selected antenna timeline", latest.txWlan, renderStepLine("txWlan"))
    )
  );
}

function healthCard(label: string, main: string, hint: string, raw: number | null, chart: SVGElement): HTMLElement {
  return el("div", { class: `health-card ${healthState(label, raw)}` },
    el("span", {}, label),
    el("strong", {}, main),
    chart,
    el("small", {}, hint)
  );
}

function healthState(label: string, value: number | null): string {
  if (value === null) {
    return "unknown";
  }
  if (label === "Packet loss") {
    return value >= 5 ? "bad" : value >= 1 ? "warn" : "good";
  }
  if (label === "SNR") {
    return value < 12 ? "bad" : value < 20 ? "warn" : "good";
  }
  if (label === "Latency") {
    return value > 25 ? "bad" : value > 10 ? "warn" : "good";
  }
  return "neutral";
}

function renderSparkline(key: keyof VideoHealthSample, options: { min: number; max: number; invert?: boolean; suffix?: string }): SVGElement {
  const points = historyPoints(key, options.min, options.max);
  const label = latestLabel(key, options.suffix ?? "");
  return sparklineSvg(points, options.invert ? "sparkline warn-line" : "sparkline", label);
}

function renderStepLine(key: keyof VideoHealthSample): SVGElement {
  const numericValues = videoHealthHistory.map((sample) => sample[key]).filter(isNumber);
  const min = Math.min(...numericValues, 0);
  const max = Math.max(...numericValues, 1);
  return sparklineSvg(historyPoints(key, min, max), "sparkline step-line", latestLabel(key, ""));
}

function sparklineSvg(points: string, className: string, label: string): SVGElement {
  return svg("svg", { class: className, viewBox: "0 0 100 32", preserveAspectRatio: "none", role: "img", "aria-label": label },
    svg("polyline", { points })
  );
}

function historyPoints(key: keyof VideoHealthSample, min: number, max: number): string {
  const rangeValue = Math.max(max - min, 1);
  const samples = videoHealthHistory.length ? videoHealthHistory : [currentVideoHealthSample(Date.now(), false)];
  return samples.map((sample, index) => {
    const raw = sample[key];
    const val = typeof raw === "number" ? raw : min;
    const x = samples.length === 1 ? 100 : (index / (samples.length - 1)) * 100;
    const y = 30 - ((Math.min(Math.max(val, min), max) - min) / rangeValue) * 28;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(" ");
}

function latestLabel(key: keyof VideoHealthSample, suffix: string): string {
  const latest = videoHealthHistory.at(-1);
  const value = latest?.[key];
  return typeof value === "number" ? `${key}: ${value.toFixed(1)}${suffix}` : `${key}: no data`;
}

function maxHistory(key: keyof VideoHealthSample): number | null {
  const values = videoHealthHistory.map((sample) => sample[key]).filter(isNumber);
  return values.length ? Math.max(...values) : null;
}

function currentVideoHealthSample(time: number, updateTxDelta: boolean): VideoHealthSample {
  const rxRows = rxEvent?.rx_ant_stats ?? [];
  const txRows = txEvent?.tx_ant_stats ?? [];
  const sent = sumNumbers(txRows.map((row) => row.pkt_sent));
  const drop = sumNumbers(txRows.map((row) => row.pkt_drop));
  return {
    time,
    snrAvg: average(rxRows.map((row) => row.snr_avg)),
    rxPackets: sumNumbers(rxRows.map((row) => row.pkt_recv)),
    dropRate: txDropRate(sent, drop, updateTxDelta),
    latencyAvg: average(txRows.map((row) => row.lat_avg)),
    txWlan: typeof rxEvent?.tx_wlan === "number" ? rxEvent.tx_wlan : null
  };
}

function txDropRate(sent: number | null, drop: number | null, updateTxDelta: boolean): number | null {
  if (sent === null || drop === null) {
    return videoHealthHistory.at(-1)?.dropRate ?? null;
  }
  if (!updateTxDelta) {
    return videoHealthHistory.at(-1)?.dropRate ?? totalDropRate(sent, drop);
  }
  const previous = previousTxTotals;
  previousTxTotals = { sent, drop };
  if (!previous || sent < previous.sent || drop < previous.drop) {
    return totalDropRate(sent, drop);
  }
  return totalDropRate(sent - previous.sent, drop - previous.drop);
}

function totalDropRate(sent: number, drop: number): number | null {
  const totalTxPackets = sent + drop;
  return totalTxPackets > 0 ? (drop / totalTxPackets) * 100 : null;
}

function recordVideoHealthSample(updateTxDelta: boolean): void {
  const sample = currentVideoHealthSample(Date.now(), updateTxDelta);
  if (sample.snrAvg === null && sample.rxPackets === null && sample.dropRate === null && sample.latencyAvg === null && sample.txWlan === null) {
    return;
  }
  videoHealthHistory = [...videoHealthHistory, sample].slice(-maxVideoHealthSamples);
}

function average(values: Array<number | undefined>): number | null {
  const numericValues = values.filter(isNumber);
  if (numericValues.length === 0) {
    return null;
  }
  return numericValues.reduce((sum, item) => sum + item, 0) / numericValues.length;
}

function sumNumbers(values: Array<number | undefined>): number | null {
  const numericValues = values.filter(isNumber);
  if (numericValues.length === 0) {
    return null;
  }
  return numericValues.reduce((sum, item) => sum + item, 0);
}

function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function formatNumber(input: number | null, suffix: string): string {
  return input === null ? "-" : `${input.toFixed(input >= 10 ? 0 : 1)}${suffix}`;
}

function percent(input: number | null): string {
  return input === null ? "-" : `${input.toFixed(input >= 10 ? 0 : 1)}%`;
}

function lossHint(input: number | null): string {
  if (input === null) {
    return "waiting for TX counters";
  }
  if (input >= 5) {
    return "likely visible artifacts";
  }
  if (input >= 1) {
    return "possible corruption bursts";
  }
  return "clean packet path";
}

function thresholdHint(input: number | null, warn: number, good: number, base: string, lowerIsBetter = false): string {
  if (input === null) {
    return "waiting for samples";
  }
  if (lowerIsBetter) {
    return input > warn ? "freeze risk" : input > good ? "watch for spikes" : base;
  }
  return input < warn ? "artifact risk" : input < good ? "watch for fades" : base;
}

function renderRXStats(): HTMLElement {
  const rows = rxEvent?.rx_ant_stats ?? [];
  return el("section", { class: "stat-section" },
    el("h3", {}, "RX Antennas"),
    table(["Ant", "Freq", "MCS", "BW", "Pkts/s", "RSSI", "SNR"], rows.map((row) => [
      value(row.ant),
      value(row.freq),
      value(row.mcs),
      value(row.bw),
      value(row.pkt_recv),
      range(row.rssi_min, row.rssi_avg, row.rssi_max),
      range(row.snr_min, row.snr_avg, row.snr_max)
    ]))
  );
}

function renderTXStats(): HTMLElement {
  const rows = txEvent?.tx_ant_stats ?? [];
  return el("section", { class: "stat-section" },
    el("h3", {}, "TX Antennas"),
    table(["Ant", "Sent", "Drop", "Latency"], rows.map((row) => [
      value(row.ant),
      value(row.pkt_sent),
      value(row.pkt_drop),
      range(row.lat_min, row.lat_avg, row.lat_max)
    ])),
    renderTemperatures()
  );
}

function renderTemperatures(): HTMLElement {
  const temps = txEvent?.rf_temperature ?? {};
  const entries = Object.entries(temps);
  if (entries.length === 0) {
    return el("p", { class: "muted" }, "No RF temperature data");
  }
  return el("div", { class: "chips" }, ...entries.map(([ant, temp]) => el("span", {}, `RF ${ant}: ${temp} C`)));
}

function renderPacketStats(): HTMLElement {
  return el("section", { class: "stat-section" },
    el("h3", {}, "Packets"),
    el("div", { class: "packet-grid" },
      packetBlock("RX", rxEvent),
      packetBlock("TX", txEvent)
    )
  );
}

function packetBlock(title: string, event: WFBStatsEvent | null): HTMLElement {
  const packets = event?.packets ?? {};
  const entries = Object.entries(packets);
  return el("div", { class: "packet-block" },
    el("h4", {}, title),
    entries.length
      ? table(["Key", "Value"], entries.map(([key, val]) => [key, formatUnknown(val)]))
      : el("p", { class: "muted" }, "No packet counters")
  );
}

function table(headers: string[], rows: string[][]): HTMLElement {
  if (rows.length === 0) {
    return el("p", { class: "muted" }, "No data");
  }
  return el("table", {},
    el("thead", {}, el("tr", {}, ...headers.map((header) => el("th", {}, header)))),
    el("tbody", {}, ...rows.map((row) => el("tr", {}, ...row.map((cell) => el("td", {}, cell)))))
  );
}

function value(input: unknown): string {
  if (input === null || input === undefined || input === "") {
    return "-";
  }
  return String(input);
}

function range(min: unknown, avg: unknown, max: unknown): string {
  if (min === undefined && avg === undefined && max === undefined) {
    return "-";
  }
  return `${value(min)} / ${value(avg)} / ${value(max)}`;
}

function formatUnknown(input: unknown): string {
  if (Array.isArray(input)) {
    return input.join(" / ");
  }
  if (input && typeof input === "object") {
    return JSON.stringify(input);
  }
  return value(input);
}

function field<T extends Record<string, string | number | boolean>>(object: T, key: keyof T, label: string, type = "text"): HTMLElement {
  return el("label", {}, label,
    el("input", {
      value: String(object[key] ?? ""),
      type,
      onInput: (event: Event) => {
        const input = event.target as HTMLInputElement;
        object[key] = (type === "number" ? Number(input.value) : input.value) as T[keyof T];
      }
    })
  );
}

function checkbox<T extends Record<string, string | number | boolean>>(object: T, key: keyof T, label: string): HTMLElement {
  return el("label", {}, label,
    el("select", {
      onChange: (event: Event) => {
        object[key] = (((event.target as HTMLSelectElement).value === "true") as T[keyof T]);
      }
    }, el("option", { value: "false", selected: String(!object[key]) }, "False"), el("option", { value: "true", selected: String(object[key]) }, "True"))
  );
}

function streamStats(): void {
  statsSource?.close();
  statsSource = new EventSource("/api/stats/stream");
  statsSource.onmessage = (event) => {
    statsConnected = true;
    applyStatsEvent(event.data);
    renderStatsIfVisible();
  };
  statsSource.onerror = () => {
    statsConnected = false;
    renderStatsIfVisible();
  };
}

function renderStatsIfVisible(): void {
  if (activeTab === "stats") {
    render();
  }
}

function applyStatsEvent(line: string): void {
  const event = JSON.parse(line) as WFBSettingsEvent | WFBStatsEvent;
  const type = event.type;
  switch (type) {
    case "settings":
      settingsEvent = event as WFBSettingsEvent;
      break;
    case "rx":
      rxEvent = event as WFBStatsEvent;
      recordVideoHealthSample(false);
      break;
    case "tx":
      txEvent = event as WFBStatsEvent;
      recordVideoHealthSample(true);
      break;
  }
}

function svg<K extends keyof SVGElementTagNameMap>(
  tag: K,
  attrs: Record<string, unknown> = {},
  ...children: Array<Node | string>
): SVGElementTagNameMap[K] {
  const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key === "class") {
      node.setAttribute("class", String(value));
    } else {
      node.setAttribute(key, String(value));
    }
  }
  for (const child of children) {
    node.append(child);
  }
  return node;
}

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, unknown> = {},
  ...children: Array<Node | string>
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key.startsWith("on") && typeof value === "function") {
      node.addEventListener(key.slice(2).toLowerCase(), value as EventListener);
    } else if (key === "class") {
      node.className = String(value);
    } else if (key === "value" && (node instanceof HTMLInputElement || node instanceof HTMLTextAreaElement || node instanceof HTMLSelectElement)) {
      node.value = String(value);
    } else if ((key === "selected" || key === "disabled" || key === "checked") && (value === false || value === "false")) {
      continue;
    } else if ((key === "selected" || key === "disabled" || key === "checked") && (value === true || value === "true")) {
      node.setAttribute(key, key);
    } else {
      node.setAttribute(key, String(value));
    }
  }
  for (const child of children) {
    node.append(child);
  }
  return node;
}

void load();
streamStats();

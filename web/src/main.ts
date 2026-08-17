import "./style.css";

type Config = {
  common: { wifi_channel: number; wifi_region: string; link_domain: string };
  base: { ldpc: number; stbc: number; bandwidth: number; mcs_index: number; force_vht: boolean };
  gs_video: { peer: string };
  default: { wfb_nics: string; rtp_mtu: number; rtp_jitter: number; rtsp_port: number; rtsp_uri: string };
};

type EffectiveConfig = {
  files: { master: string; local: string; default: string };
  sections: Array<{ name: string; fields: EffectiveField[] }>;
};

type EffectiveField = {
  section: string;
  key: string;
  value: string;
  default: boolean;
  source: string;
};

type ServiceState = {
  unit: string;
  active: string;
  sub: string;
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

const root = document.querySelector<HTMLDivElement>("#app");
if (!root) {
  throw new Error("missing app element");
}
const app = root;

let config: Config | null = null;
let effectiveConfig: EffectiveConfig | null = null;
let profileSelection: ProfileSelection | null = null;
let services: ServiceState[] = [];
let radios: RadioInfo[] = [];
let keyInfo: KeyInfo | null = null;
let settingsEvent: WFBSettingsEvent | null = null;
let rxEvent: WFBStatsEvent | null = null;
let txEvent: WFBStatsEvent | null = null;
let statsConnected = false;
let error = "";
let activeTab = "stats";
let statsSource: EventSource | null = null;

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
  if (!config) {
    return;
  }
  try {
    config = await requestJSON<Config>("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config)
    });
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

function renderServices(): HTMLElement {
  return el("div", { class: "panel" },
    el("h2", {}, "Services"),
    ...services.map((service) =>
      el("p", {},
        el("span", { class: `status-dot ${service.active === "active" ? "active" : ""}` }),
        `${service.unit}: ${service.active}/${service.sub}`
      )
    ),
    el("div", { class: "actions" },
      el("button", { onClick: () => serviceAction("wifibroadcast-gs", "restart") }, "Restart GS"),
      el("button", { class: "secondary", onClick: () => serviceAction("wifibroadcast-gs", "start") }, "Start GS"),
      el("button", { class: "secondary", onClick: () => serviceAction("wifibroadcast-gs", "stop") }, "Stop GS"),
      el("button", { class: "secondary", onClick: () => serviceAction("rtsp-h265", "restart") }, "Restart H265 RTSP"),
      el("button", { class: "secondary", onClick: () => serviceAction("rtsp-h264", "restart") }, "Restart H264 RTSP"),
      el("button", { class: "secondary", onClick: () => serviceAction("fpv-camera", "restart") }, "Restart FPV Camera")
    )
  );
}

function renderConfig(): HTMLElement {
  if (!config || !effectiveConfig) {
    return el("div", { class: "panel" }, "No config loaded");
  }
  return el("div", { class: "panel" },
    el("h2", {}, "Configuration"),
    el("div", { class: "file-grid" },
      fileBadge("master.cfg", effectiveConfig.files.master || "not found"),
      fileBadge("wifibroadcast.cfg", effectiveConfig.files.local),
      fileBadge("default", effectiveConfig.files.default)
    ),
    ...effectiveConfig.sections.map(renderConfigSection)
  );
}

function renderConfigSection(section: { name: string; fields: EffectiveField[] }): HTMLElement {
  return el("section", { class: "config-section" },
    el("h3", {}, `[${section.name}]`),
    table(["Default", "Key", "Value", "Source"], section.fields.map((fieldInfo) => [
      fieldInfo.default ? "yes" : "no",
      fieldInfo.key,
      fieldInfo.value,
      fieldInfo.source
    ]))
  );
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
    render();
  };
  statsSource.onerror = () => {
    statsConnected = false;
    render();
  };
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
      break;
    case "tx":
      txEvent = event as WFBStatsEvent;
      break;
  }
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
    } else if (key === "selected" && value === "false") {
      continue;
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

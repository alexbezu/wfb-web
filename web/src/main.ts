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
let configSearch = "";
let configSection = "all";
let configChangedOnly = false;
let configDrafts = new Map<string, string>();

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
  return el("tr", {},
    el("td", {},
      el("span", { class: `status-dot ${service.active === "active" ? "active" : ""}` }),
      service.unit
    ),
    el("td", {},
      el("div", { class: "row-actions" },
        el("button", { class: "secondary compact", onClick: () => serviceAction(key, "start") }, "Start"),
        el("button", { class: "secondary compact", onClick: () => serviceAction(key, "stop") }, "Stop"),
        el("button", { class: "compact", onClick: () => serviceAction(key, "restart") }, "Restart")
      )
    ),
    el("td", {}, `${service.active}/${service.sub}`),
    el("td", {}, service.load || "-")
  );
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
  const standard = [
    ["common", "wifi_channel", "WiFi Channel"],
    ["common", "wifi_region", "WiFi Region"],
    ["common", "wifi_txpower", "TX Power"],
    ["common", "link_domain", "Link Domain"],
    ["base", "bandwidth", "Bandwidth"],
    ["base", "mcs_index", "MCS Index"],
    ["base", "ldpc", "LDPC"],
    ["base", "stbc", "STBC"],
    ["base", "force_vht", "Force VHT"],
    ["gs_video", "peer", "GS Video Peer"],
    ["default", "WFB_NICS", "WFB NICS"],
    ["default", "RTP_MTU", "RTP MTU"],
    ["default", "RTP_JITTER", "RTP Jitter"],
    ["default", "RTSP_PORT", "RTSP Port"],
    ["default", "RTSP_URI", "RTSP URI"]
  ];
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

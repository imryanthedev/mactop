import { formatKibPerSec, formatPercent, formatWatts } from "./format.js";
import { renderSimple, setSimpleLoading } from "./views/simple.js";
import { getActiveView, initTabs, PHASE } from "./views/tabs.js";

const statusEl = document.getElementById("status");

function setStatus(message, isError = false) {
  if (!message) {
    statusEl.hidden = true;
    statusEl.textContent = "";
    statusEl.classList.remove("error");
    return;
  }

  statusEl.hidden = false;
  statusEl.textContent = message;
  statusEl.classList.toggle("error", isError);
}

function renderAdvanced(metrics) {
  const soc = metrics.soc_metrics || {};
  const netDisk = metrics.net_disk || {};
  const processes = metrics.processes || [];

  document.getElementById("advanced-soc").textContent = [
    `Chip: ${metrics.system_info?.name || "—"}`,
    `CPU power: ${formatWatts(soc.cpu_power)}`,
    `GPU power: ${formatWatts(soc.gpu_power)}`,
    `ANE power: ${formatWatts(soc.ane_power)}`,
    `DRAM power: ${formatWatts(soc.dram_power)}`,
    `GPU freq: ${soc.gpu_freq_mhz ? `${soc.gpu_freq_mhz} MHz` : "—"}`,
    `Cores tracked: ${metrics.core_usages?.length || 0}`,
  ].join("\n");

  document.getElementById("advanced-io").textContent = [
    `Net in: ${formatKibPerSec(netDisk.in_bytes_per_sec, "byte")}`,
    `Net out: ${formatKibPerSec(netDisk.out_bytes_per_sec, "byte")}`,
    `Disk read: ${formatKibPerSec(netDisk.read_kbytes_per_sec, "kib")}`,
    `Disk write: ${formatKibPerSec(netDisk.write_kbytes_per_sec, "kib")}`,
  ].join("\n");

  if (processes.length === 0) {
    document.getElementById("advanced-processes").textContent = "No process data.";
    return;
  }

  document.getElementById("advanced-processes").textContent = processes
    .slice(0, 10)
    .map((proc) => `${proc.pid}\t${formatPercent(proc.cpu_percent)}\t${proc.command}`)
    .join("\n");
}

function renderComplex(metrics) {
  document.getElementById("complex-json").textContent = JSON.stringify(metrics, null, 2);
}

function renderMetrics(metrics) {
  if (!metrics) {
    return;
  }

  setStatus("");
  setSimpleLoading(false);
  renderSimple(metrics);

  if (PHASE.advanced && getActiveView() === "advanced") {
    renderAdvanced(metrics);
  }
  if (PHASE.complex && getActiveView() === "complex") {
    renderComplex(metrics);
  }
}

initTabs();

if (window.mactop) {
  setSimpleLoading(true);

  window.mactop.onMetricsUpdate(renderMetrics);
  window.mactop.onMetricsError((message) => setStatus(`Sidecar error: ${message}`, true));
} else {
  setStatus("Desktop bridge unavailable. Run inside Electron.", true);
}

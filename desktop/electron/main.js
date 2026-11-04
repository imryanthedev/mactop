const { app, BrowserWindow, net, protocol } = require("electron");
const { spawn } = require("child_process");
const path = require("path");
const { pathToFileURL } = require("url");

const RENDERER_SCHEME = "app";
const RENDERER_HOST = "mactop";
const SIDECAR_ARGS = ["--headless"]; // infinite samples: one JSON object per line
const SIDECAR_RESTART_DELAY_MS = 2000;
const MAX_SIDECAR_RESTARTS = 5;
const MAX_STDOUT_BUFFER_BYTES = 4 * 1024 * 1024;

let mainWindow = null;
let sidecarProcess = null;
let sidecarRestartTimer = null;
let sidecarRestarts = 0;
let stdoutBuffer = "";

// The renderer is ES modules, and Chromium refuses module scripts loaded over
// file:// (opaque origin, CORS). Serving src/ from a standard, secure scheme
// keeps the renderer modular without disabling webSecurity.
protocol.registerSchemesAsPrivileged([
  {
    scheme: RENDERER_SCHEME,
    privileges: { standard: true, secure: true, supportFetchAPI: true },
  },
]);

const CONTENT_TYPES = {
  ".css": "text/css",
  ".html": "text/html",
  ".js": "text/javascript",
  ".json": "application/json",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
};

function registerRendererProtocol() {
  const srcRoot = path.join(__dirname, "..", "src");

  protocol.handle(RENDERER_SCHEME, async (request) => {
    const { host, pathname } = new URL(request.url);
    if (host !== RENDERER_HOST) {
      return new Response("Not found", { status: 404 });
    }

    const relativePath = pathname === "/" ? "index.html" : pathname.slice(1);
    const filePath = path.join(srcRoot, relativePath);

    if (!filePath.startsWith(srcRoot + path.sep)) {
      return new Response("Forbidden", { status: 403 });
    }

    const response = await net.fetch(pathToFileURL(filePath).toString());

    // Chromium refuses module scripts without a JavaScript content type, and
    // not every platform reports one for file: responses.
    const contentType = CONTENT_TYPES[path.extname(filePath).toLowerCase()];
    if (!contentType) {
      return response;
    }

    const headers = new Headers(response.headers);
    headers.set("content-type", contentType);
    return new Response(response.body, { status: response.status, headers });
  });
}

function sidecarBinaryPath() {
  if (process.env.MACTOP_BINARY) {
    return process.env.MACTOP_BINARY;
  }

  if (app.isPackaged) {
    return path.join(process.resourcesPath, "mactop");
  }

  return path.resolve(__dirname, "..", "..", "mactop");
}

function send(channel, payload) {
  if (!mainWindow || mainWindow.isDestroyed()) {
    return;
  }
  mainWindow.webContents.send(channel, payload);
}

function handleStdoutLine(line) {
  const trimmed = line.trim();
  if (!trimmed) {
    return;
  }

  try {
    send("metrics:update", JSON.parse(trimmed));
  } catch (err) {
    send("metrics:error", `Unreadable sample: ${err.message}`);
  }
}

function consumeStdout(chunk) {
  stdoutBuffer += chunk;

  // A malformed stream must not grow without bound.
  if (stdoutBuffer.length > MAX_STDOUT_BUFFER_BYTES) {
    stdoutBuffer = "";
    send("metrics:error", "Sidecar produced more than 4 MiB without a newline.");
    return;
  }

  let newlineIndex = stdoutBuffer.indexOf("\n");
  while (newlineIndex !== -1) {
    handleStdoutLine(stdoutBuffer.slice(0, newlineIndex));
    stdoutBuffer = stdoutBuffer.slice(newlineIndex + 1);
    newlineIndex = stdoutBuffer.indexOf("\n");
  }
}

function stopSidecar() {
  if (sidecarRestartTimer) {
    clearTimeout(sidecarRestartTimer);
    sidecarRestartTimer = null;
  }

  if (!sidecarProcess) {
    return;
  }

  const child = sidecarProcess;
  sidecarProcess = null;
  stdoutBuffer = "";

  child.removeAllListeners("close");
  child.kill();
}

function scheduleSidecarRestart(reason) {
  if (sidecarRestartTimer || !mainWindow || mainWindow.isDestroyed()) {
    return;
  }

  if (sidecarRestarts >= MAX_SIDECAR_RESTARTS) {
    send(
      "metrics:error",
      `Sidecar stopped ${sidecarRestarts} times (${reason}). Set MACTOP_BINARY and restart the app.`,
    );
    return;
  }

  sidecarRestarts += 1;
  sidecarRestartTimer = setTimeout(() => {
    sidecarRestartTimer = null;
    startSidecar();
  }, SIDECAR_RESTART_DELAY_MS);
}

function startSidecar() {
  if (sidecarProcess) {
    return;
  }

  const binary = sidecarBinaryPath();
  stdoutBuffer = "";

  const child = spawn(binary, SIDECAR_ARGS, {
    cwd: path.resolve(__dirname, "..", ".."),
  });
  sidecarProcess = child;

  child.stdout.setEncoding("utf8");
  child.stdout.on("data", consumeStdout);

  child.stderr.setEncoding("utf8");
  child.stderr.on("data", (chunk) => {
    const message = chunk.trim();
    if (message) {
      send("metrics:error", message);
    }
  });

  child.on("error", (err) => {
    if (sidecarProcess === child) {
      sidecarProcess = null;
    }
    send("metrics:error", `Cannot start sidecar (${binary}): ${err.message}`);
    scheduleSidecarRestart("spawn error");
  });

  child.on("close", (code) => {
    if (sidecarProcess !== child) {
      return;
    }
    sidecarProcess = null;
    send("metrics:error", `Sidecar exited with code ${code}.`);
    scheduleSidecarRestart(`exit ${code}`);
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1100,
    height: 720,
    minWidth: 960,
    minHeight: 600,
    title: "mactop-desktop",
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  mainWindow.loadURL(`${RENDERER_SCHEME}://${RENDERER_HOST}/index.html`);

  mainWindow.on("closed", () => {
    mainWindow = null;
    stopSidecar();
  });

  mainWindow.webContents.on("did-finish-load", () => {
    sidecarRestarts = 0;
    startSidecar();
  });
}

app.whenReady().then(() => {
  registerRendererProtocol();
  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("before-quit", () => {
  stopSidecar();
});

app.on("window-all-closed", () => {
  stopSidecar();
  if (process.platform !== "darwin") {
    app.quit();
  }
});

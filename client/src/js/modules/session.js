import { invoke } from "@tauri-apps/api/core";
import { state } from "./state.js";
import { wsClient } from "./ws.js";

const runtime = {
  stream: null,
  video: null,
  canvas: null,
  timer: null,
  captureInFlight: false,
  nativeFailures: 0,
  mode: null,
  inputHandlers: []
};

export async function startHostSharing(log, options = {}) {
  if (runtime.timer) {
    return;
  }

  const forceBrowser = options.forceBrowser === true;
  runtime.nativeFailures = 0;

  if (!forceBrowser) {
    const nativeStarted = await tryStartNativeCapture(log);
    if (nativeStarted) {
      return;
    }
  }

  await startBrowserCapture(log);
}

async function tryStartNativeCapture(log) {
  try {
    // probe command availability and screen access
    await invoke("capture_primary_screen_jpeg", { quality: 60 });
  } catch {
    return false;
  }

  runtime.mode = "native";
  runtime.timer = setInterval(async () => {
    if (!state.activeSession || runtime.captureInFlight) {
      return;
    }

    runtime.captureInFlight = true;
    try {
      const imageData = await invoke("capture_primary_screen_jpeg", { quality: 60 });
      wsClient.sendSessionSignal("frame", {
        image_data: imageData,
        width: 0,
        height: 0,
        ts: Date.now()
      });
      runtime.nativeFailures = 0;
    } catch (error) {
      runtime.nativeFailures += 1;
      if (runtime.nativeFailures >= 4) {
        clearInterval(runtime.timer);
        runtime.timer = null;
        runtime.captureInFlight = false;
        runtime.nativeFailures = 0;
        log(`Falha na captura nativa (${error?.message || error}). Voltando para captura via sistema.`);
        startHostSharing(log, { forceBrowser: true }).catch((fallbackError) => {
          log(`Falha no fallback de captura: ${fallbackError?.message || fallbackError}`);
        });
        return;
      }
    } finally {
      runtime.captureInFlight = false;
    }
  }, 160);

  log("Compartilhamento de tela iniciado automaticamente (captura nativa da tela inteira)");
  return true;
}

async function startBrowserCapture(log) {
  runtime.stream = await navigator.mediaDevices.getDisplayMedia({
    video: { frameRate: 8 },
    audio: false
  });

  runtime.video = document.createElement("video");
  runtime.video.srcObject = runtime.stream;
  runtime.video.muted = true;
  await runtime.video.play();

  runtime.canvas = document.createElement("canvas");
  const ctx = runtime.canvas.getContext("2d", { alpha: false });

  runtime.mode = "browser";
  runtime.timer = setInterval(() => {
    if (!state.activeSession || !ctx || !runtime.video) {
      return;
    }

    const width = Math.max(960, runtime.video.videoWidth || 960);
    const height = Math.max(540, runtime.video.videoHeight || 540);
    runtime.canvas.width = width;
    runtime.canvas.height = height;
    ctx.drawImage(runtime.video, 0, 0, width, height);

    const imageData = runtime.canvas.toDataURL("image/jpeg", 0.62);
    wsClient.sendSessionSignal("frame", {
      image_data: imageData,
      width,
      height,
      ts: Date.now()
    });
  }, 220);

  runtime.stream.getVideoTracks()[0].addEventListener("ended", () => {
    stopHostSharing(log);
  });

  log("Compartilhamento de tela iniciado (fallback do sistema)");
}

export function stopHostSharing(log) {
  if (runtime.timer) {
    clearInterval(runtime.timer);
    runtime.timer = null;
  }
  if (runtime.stream) {
    runtime.stream.getTracks().forEach((track) => track.stop());
    runtime.stream = null;
  }
  runtime.video = null;
  runtime.canvas = null;
  runtime.mode = null;
  runtime.captureInFlight = false;
  runtime.nativeFailures = 0;
  log("Compartilhamento de tela encerrado");
}

export function bindControllerInput(frameEl, log) {
  unbindControllerInput();
  frameEl.tabIndex = 0;
  frameEl.style.cursor = "crosshair";

  let lastMoveSentAt = 0;

  const sendMouse = (eventType, event) => {
    const rect = frameEl.getBoundingClientRect();
    if (!rect.width || !rect.height) {
      return;
    }
    const x = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
    const y = Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height));
    wsClient.sendSessionSignal("input", {
      event_type: eventType,
      x,
      y,
      button: event.button,
      key: null
    });
  };

  const onMove = (e) => {
    const now = Date.now();
    if (now - lastMoveSentAt < 33) {
      return;
    }
    lastMoveSentAt = now;
    sendMouse("mouse_move", e);
  };
  const onDown = (e) => {
    e.preventDefault();
    frameEl.focus();
    sendMouse("mouse_down", e);
  };
  const onUp = (e) => {
    e.preventDefault();
    sendMouse("mouse_up", e);
  };
  const onKeyDown = (e) => {
    e.preventDefault();
    wsClient.sendSessionSignal("input", {
      event_type: "key_down",
      key: e.key,
      code: e.code
    });
  };
  const onKeyUp = (e) => {
    e.preventDefault();
    wsClient.sendSessionSignal("input", {
      event_type: "key_up",
      key: e.key,
      code: e.code
    });
  };
  const onContextMenu = (e) => e.preventDefault();
  const onWheel = (e) => {
    e.preventDefault();
    wsClient.sendSessionSignal("input", {
      event_type: "mouse_wheel",
      delta_x: e.deltaX,
      delta_y: e.deltaY
    });
  };

  frameEl.addEventListener("mousemove", onMove);
  frameEl.addEventListener("mousedown", onDown);
  frameEl.addEventListener("mouseup", onUp);
  frameEl.addEventListener("keydown", onKeyDown);
  frameEl.addEventListener("keyup", onKeyUp);
  frameEl.addEventListener("contextmenu", onContextMenu);
  frameEl.addEventListener("wheel", onWheel, { passive: false });

  runtime.inputHandlers = [
    ["mousemove", onMove],
    ["mousedown", onDown],
    ["mouseup", onUp],
    ["keydown", onKeyDown],
    ["keyup", onKeyUp],
    ["contextmenu", onContextMenu],
    ["wheel", onWheel]
  ];

  log("Captura de input remoto habilitada (controller). Clique na tela remota para focar teclado.");
}

export function unbindControllerInput() {
  const frameEl = document.getElementById("remoteFrame");
  if (!frameEl) {
    return;
  }
  for (const [eventName, handler] of runtime.inputHandlers) {
    frameEl.removeEventListener(eventName, handler);
  }
  runtime.inputHandlers = [];
}

export async function handleSessionSignal(message, log, setRemoteFrame) {
  const payload = message.payload || {};

  if (message.kind === "frame") {
    if (payload.image_data) {
      setRemoteFrame(payload.image_data);
    }
    return;
  }

  if (message.kind === "input") {
    try {
      await invoke("apply_input_event", { event: payload });
    } catch (error) {
      log(`Falha ao aplicar input no host: ${error.message || error}`);
    }
  }
}

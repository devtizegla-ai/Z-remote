import { invoke } from "@tauri-apps/api/core";
import { state } from "./state.js";
import { wsClient } from "./ws.js";

const runtime = {
  stream: null,
  video: null,
  canvas: null,
  timer: null,
  inputHandlers: []
};

export async function startHostSharing(log) {
  if (runtime.timer) {
    return;
  }
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

  log("Compartilhamento de tela iniciado (host)");
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
  log("Compartilhamento de tela encerrado");
}

export function bindControllerInput(frameEl, log) {
  unbindControllerInput();
  frameEl.tabIndex = 0;

  const sendMouse = (eventType, event) => {
    const rect = frameEl.getBoundingClientRect();
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

  const onMove = (e) => sendMouse("mouse_move", e);
  const onDown = (e) => sendMouse("mouse_down", e);
  const onUp = (e) => sendMouse("mouse_up", e);
  const onKeyDown = (e) => {
    wsClient.sendSessionSignal("input", {
      event_type: "key_down",
      key: e.key,
      code: e.code
    });
  };
  const onKeyUp = (e) => {
    wsClient.sendSessionSignal("input", {
      event_type: "key_up",
      key: e.key,
      code: e.code
    });
  };

  frameEl.addEventListener("mousemove", onMove);
  frameEl.addEventListener("mousedown", onDown);
  frameEl.addEventListener("mouseup", onUp);
  frameEl.addEventListener("keydown", onKeyDown);
  frameEl.addEventListener("keyup", onKeyUp);

  runtime.inputHandlers = [
    ["mousemove", onMove],
    ["mousedown", onDown],
    ["mouseup", onUp],
    ["keydown", onKeyDown],
    ["keyup", onKeyUp]
  ];

  log("Captura de input remoto habilitada (controller)");
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


import { invoke } from "@tauri-apps/api/core";
import { apiRequest } from "./modules/api.js";
import {
  clearTokens,
  clearUser,
  getDeviceId,
  loadSettings,
  loadTokens,
  loadUser,
  saveSettings,
  saveTokens,
  saveUser
} from "./modules/config.js";
import { downloadTransfer, uploadSessionFile } from "./modules/files.js";
import { bindControllerInput, handleSessionSignal, startHostSharing, stopHostSharing, unbindControllerInput } from "./modules/session.js";
import { setState, state, subscribe } from "./modules/state.js";
import { createUI } from "./modules/ui.js";
import { wsClient } from "./modules/ws.js";

const ui = createUI();

setState({
  settings: loadSettings(),
  tokens: loadTokens(),
  user: loadUser(),
  serverReachable: null
});

ui.showTab("login");
subscribe(() => {
  ui.render();
  ui.renderIncomingFiles(onDownloadTransfer);
});
ui.render();

bindBaseEvents();
bindWSHandlers();
startServerHealthMonitor();

if (state.tokens?.access_token) {
  bootstrapWithExistingToken();
}

async function bootstrapWithExistingToken() {
  try {
    const me = await apiRequest("/api/me", { method: "GET" });
    setState({ user: me });
    saveUser(me);
    await registerCurrentDevice();
    await refreshDevices();
  } catch (error) {
    ui.log(`Sessão salva inválida: ${error.message}`);
    logout();
  }
}

function bindBaseEvents() {
  ui.elements.tabLogin.addEventListener("click", () => ui.showTab("login"));
  ui.elements.tabRegister.addEventListener("click", () => ui.showTab("register"));
  ui.elements.authSettingsBtn.addEventListener("click", () => ui.showSettings(true));

  ui.elements.loginForm.addEventListener("submit", onLogin);
  ui.elements.registerForm.addEventListener("submit", onRegister);
  ui.elements.refreshDevicesBtn.addEventListener("click", refreshDevices);
  ui.elements.logoutBtn.addEventListener("click", logout);

  ui.elements.startShareBtn.addEventListener("click", async () => {
    try {
      await startHostSharing(ui.log);
    } catch (error) {
      ui.log(`Falha ao iniciar compartilhamento: ${error.message || error}`);
    }
  });
  ui.elements.stopShareBtn.addEventListener("click", () => stopHostSharing(ui.log));

  ui.elements.sendFileBtn.addEventListener("click", onSendFile);

  ui.elements.acceptRequestBtn.addEventListener("click", () => onRespondRequest(true));
  ui.elements.rejectRequestBtn.addEventListener("click", () => onRespondRequest(false));

  ui.elements.openSettingsBtn.addEventListener("click", () => ui.showSettings(true));
  ui.elements.closeSettingsBtn.addEventListener("click", () => ui.showSettings(false));
  ui.elements.saveSettingsBtn.addEventListener("click", onSaveSettings);
}

function bindWSHandlers() {
  wsClient.on("open", () => ui.log("Canal WebSocket conectado"));
  wsClient.on("close", () => ui.log("Canal WebSocket desconectado"));
  wsClient.on("error", (error) => ui.log(error.message));

  wsClient.on("session_request", ({ request }) => {
    if (!request || request.target_device_id !== state.device?.id) {
      return;
    }
    setState({ pendingRequest: request });
    ui.log(`Solicitação recebida: ${request.id}`);
  });

  wsClient.on("session_response", async ({ request }) => {
    if (!request || request.requester_device_id !== state.device?.id) {
      return;
    }
    ui.log(`Solicitação ${request.id} ${request.status}`);
    if (request.status === "accepted") {
      try {
        const started = await apiRequest("/api/sessions/start", {
          method: "POST",
          body: JSON.stringify({
            request_id: request.id,
            requester_device_id: state.device.id
          })
        });
        setState({ activeSession: started.remote_session || started.session || null });
      } catch (error) {
        ui.log(`Falha ao iniciar sessão: ${error.message}`);
      }
    }
  });

  wsClient.on("session_started", ({ session }) => {
    if (!session) {
      return;
    }
    setState({ activeSession: session });
    ui.log(`Sessão ativa: ${session.id}`);

    const isController = session.requester_device_id === state.device?.id;
    if (isController) {
      bindControllerInput(ui.elements.remoteFrame, ui.log);
    } else {
      unbindControllerInput();
    }
  });

  wsClient.on("session_signal", async (message) => {
    await handleSessionSignal(message, ui.log, ui.setRemoteFrame);
  });

  wsClient.on("file_available", ({ file }) => {
    if (!file) {
      return;
    }
    setState({ incomingFiles: [file, ...state.incomingFiles].slice(0, 20) });
    ui.log(`Arquivo recebido: ${file.filename}`);
  });

  wsClient.on("error", ({ payload }) => {
    if (payload?.message) {
      ui.log(`Erro WS: ${payload.message}`);
    }
  });

  setInterval(() => {
    wsClient.send("heartbeat", {});
  }, 20000);
}

async function onLogin(event) {
  event.preventDefault();
  await checkServerHealth();
  try {
    const payload = await apiRequest("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({
        email: ui.elements.loginEmail.value.trim(),
        password: ui.elements.loginPassword.value
      })
    });

    setState({ tokens: payload, user: payload.user });
    saveTokens(payload);
    saveUser(payload.user);

    await registerCurrentDevice();
    await refreshDevices();

    ui.log("Login realizado");
  } catch (error) {
    ui.log(`Falha no login: ${humanizeNetworkError(error)}`);
  }
}

async function onRegister(event) {
  event.preventDefault();
  await checkServerHealth();
  try {
    await apiRequest("/api/auth/register", {
      method: "POST",
      body: JSON.stringify({
        name: ui.elements.registerName.value.trim(),
        email: ui.elements.registerEmail.value.trim(),
        password: ui.elements.registerPassword.value
      })
    });
    ui.log("Cadastro concluído. Faça login para continuar.");
    ui.showTab("login");
  } catch (error) {
    ui.log(`Falha no cadastro: ${humanizeNetworkError(error)}`);
  }
}

async function registerCurrentDevice() {
  const deviceId = getDeviceId();
  let runtimeInfo = { platform: navigator.platform || "unknown", app_version: import.meta.env.VITE_APP_VERSION || "0.1.0" };
  try {
    runtimeInfo = await invoke("get_runtime_info");
  } catch {
    // fallback em modo navegador
  }

  const registered = await apiRequest("/api/devices/register", {
    method: "POST",
    headers: {
      "X-Device-ID": deviceId
    },
    body: JSON.stringify({
      device_id: deviceId,
      device_name: state.settings.deviceName,
      platform: runtimeInfo.platform || "unknown",
      app_version: runtimeInfo.app_version || import.meta.env.VITE_APP_VERSION || "0.1.0"
    })
  });

  setState({ device: registered });
  wsClient.disconnect();
  wsClient.connect();
}

async function refreshDevices() {
  if (!state.tokens?.access_token) {
    return;
  }
  try {
    const result = await apiRequest("/api/devices", { method: "GET" });
    setState({ devices: result.devices || [] });
    ui.renderDevices(onConnectToDevice);
  } catch (error) {
    ui.log(`Falha ao atualizar dispositivos: ${error.message}`);
  }
}

async function onConnectToDevice(device) {
  try {
    const response = await apiRequest("/api/sessions/request", {
      method: "POST",
      body: JSON.stringify({
        requester_device_id: state.device.id,
        target_device_id: device.id
      })
    });
    const request = response.session_request;
    ui.log(`Solicitação enviada para ${device.device_name} (${request.id})`);
  } catch (error) {
    ui.log(`Falha ao solicitar conexão: ${error.message}`);
  }
}

async function onRespondRequest(accept) {
  if (!state.pendingRequest) {
    return;
  }
  try {
    await apiRequest("/api/sessions/respond", {
      method: "POST",
      body: JSON.stringify({
        request_id: state.pendingRequest.id,
        target_device_id: state.device.id,
        accept
      })
    });
    ui.log(`Solicitação ${accept ? "aceita" : "rejeitada"}`);
  } catch (error) {
    ui.log(`Erro ao responder solicitação: ${error.message}`);
  } finally {
    setState({ pendingRequest: null });
  }
}

async function onSendFile() {
  const file = ui.elements.fileInput.files?.[0];
  if (!file) {
    ui.log("Selecione um arquivo antes do envio");
    return;
  }
  try {
    ui.updateUploadProgress(0);
    await uploadSessionFile(file, (pct) => ui.updateUploadProgress(pct));
    ui.log(`Arquivo enviado: ${file.name}`);
  } catch (error) {
    ui.log(`Falha no envio de arquivo: ${error.message}`);
  }
}

async function onDownloadTransfer(transferId) {
  try {
    await downloadTransfer(transferId);
    ui.log(`Arquivo baixado: ${transferId}`);
  } catch (error) {
    ui.log(`Falha no download: ${error.message}`);
  }
}

function onSaveSettings() {
  const updated = {
    serverUrl: ui.elements.settingsServerUrl.value.trim(),
    deviceName: ui.elements.settingsDeviceName.value.trim(),
    autoStartPrepared: ui.elements.settingsAutoStart.checked
  };
  saveSettings(updated);
  setState({ settings: updated });
  ui.showSettings(false);
  ui.log("Configurações salvas");
  checkServerHealth();
}

function logout() {
  stopHostSharing(ui.log);
  unbindControllerInput();
  wsClient.disconnect();

  clearTokens();
  clearUser();
  setState({
    tokens: null,
    user: null,
    device: null,
    devices: [],
    wsConnected: false,
    pendingRequest: null,
    activeSession: null,
    incomingFiles: []
  });

  ui.log("Sessão encerrada");
}

function startServerHealthMonitor() {
  checkServerHealth();
  setInterval(checkServerHealth, 20000);
}

async function checkServerHealth() {
  try {
    await apiRequest("/health", { method: "GET", retry: false, timeoutMs: 20000 });
    setState({ serverReachable: true });
    return true;
  } catch {
    setState({ serverReachable: false });
    return false;
  }
}

function humanizeNetworkError(error) {
  const message = error?.message || String(error);
  if (message.includes("Failed to fetch") || message.includes("NetworkError") || message.includes("abort")) {
    return "Servidor indisponível ou iniciando (Render free pode levar até ~1 min). Tente novamente.";
  }
  return message;
}


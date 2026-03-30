import { invoke } from "@tauri-apps/api/core";
import { apiRequest, setInvalidDeviceAuthHandler } from "./modules/api.js";
import {
  clearTokens,
  clearUser,
  getDeviceAuthKey,
  getDeviceId,
  loadSettings,
  loadTokens,
  loadUser,
  normalizeServerUrl,
  resetDeviceIdentity,
  saveSettings,
  saveTokens,
  saveUser
} from "./modules/config.js";
import { downloadTransfer, uploadSessionFile } from "./modules/files.js";
import {
  bindControllerInput,
  handleSessionSignal,
  startHostSharing,
  stopHostSharing,
  unbindControllerInput
} from "./modules/session.js";
import { setState, state, subscribe } from "./modules/state.js";
import { createUI } from "./modules/ui.js";
import { wsClient } from "./modules/ws.js";

const ui = createUI();
let recoveringDeviceIdentity = false;
let lastDeviceRecoveryAt = 0;
let wsClose1006Count = 0;
let bootstrappingAuth = false;

setState({
  settings: loadSettings(),
  tokens: loadTokens(),
  user: loadUser(),
  serverReachable: null,
  bootMessage: "Inicializando dispositivo..."
});

subscribe(() => {
  ui.render();
  ui.renderIncomingFiles(onDownloadTransfer);
});
ui.render();

bindBaseEvents();
bindWSHandlers();
bindDeviceRecoveryHandler();
startServerHealthMonitor();
bootstrapDeviceAuth(false);

function bindBaseEvents() {
  ui.elements.authSettingsBtn.addEventListener("click", () => ui.showSettings(true));
  ui.elements.retryBootstrapBtn.addEventListener("click", () => bootstrapDeviceAuth(false));

  ui.elements.refreshDevicesBtn.addEventListener("click", refreshDevices);
  ui.elements.logoutBtn.addEventListener("click", onReprovisionDevice);
  ui.elements.connectByIdBtn.addEventListener("click", onConnectById);
  ui.elements.partnerIdInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      onConnectById();
    }
  });
  ui.elements.copyDeviceIdBtn.addEventListener("click", onCopyMyId);

  ui.elements.startShareBtn.addEventListener("click", async () => {
    try {
      await startHostSharing(ui.log);
    } catch (error) {
      ui.log(`Falha ao iniciar compartilhamento: ${error.message || error}`);
    }
  });
  ui.elements.stopShareBtn.addEventListener("click", () => stopHostSharing(ui.log));
  ui.elements.endSessionBtn.addEventListener("click", onEndSession);

  ui.elements.sendFileBtn.addEventListener("click", onSendFile);
  ui.elements.acceptRequestBtn.addEventListener("click", () => onRespondRequest(true));
  ui.elements.rejectRequestBtn.addEventListener("click", () => onRespondRequest(false));

  ui.elements.openSettingsBtn.addEventListener("click", () => ui.showSettings(true));
  ui.elements.closeSettingsBtn.addEventListener("click", () => ui.showSettings(false));
  ui.elements.saveSettingsBtn.addEventListener("click", onSaveSettings);
}

function bindWSHandlers() {
  wsClient.on("open", () => {
    wsClose1006Count = 0;
    ui.log("Canal WebSocket conectado");
  });

  wsClient.on("close", ({ code, reason } = {}) => {
    const suffix = code ? ` (code=${code}${reason ? `, reason=${reason}` : ""})` : "";
    ui.log(`Canal WebSocket desconectado${suffix}`);
    if (code === 1006) {
      wsClose1006Count += 1;
      if (wsClose1006Count >= 3) {
        wsClose1006Count = 0;
        recoverDeviceIdentity("WebSocket 1006 repetido");
      }
    } else {
      wsClose1006Count = 0;
    }
  });

  wsClient.on("error", (error) => ui.log(error.message));

  wsClient.on("session_request", ({ request }) => {
    if (!request || request.target_device_id !== state.device?.id) {
      return;
    }
    setState({ pendingRequest: request });
    ui.log(`Solicitacao recebida: ${request.id}`);
  });

  wsClient.on("session_response", async ({ request }) => {
    if (!request || request.requester_device_id !== state.device?.id) {
      return;
    }
    ui.log(`Solicitacao ${request.id} ${request.status}`);
    if (request.status === "accepted") {
      try {
        const started = await apiRequest("/api/sessions/start", {
          method: "POST",
          body: JSON.stringify({ request_id: request.id })
        });
        setState({ activeSession: started.remote_session || started.session || null });
      } catch (error) {
        ui.log(`Falha ao iniciar sessao: ${error.message}`);
      }
    }
  });

  wsClient.on("session_started", ({ session }) => {
    if (!session) {
      return;
    }
    setState({ activeSession: session });
    ui.log(`Sessao ativa: ${session.id}`);

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

  wsClient.on("session_ended", ({ session, ended_by_device_id: endedBy } = {}) => {
    if (!session?.id || !state.activeSession || state.activeSession.id !== session.id) {
      return;
    }
    cleanupActiveSession(`Sessao encerrada por ${endedBy || "peer"}`);
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

function bindDeviceRecoveryHandler() {
  setInvalidDeviceAuthHandler(async () => {
    await recoverDeviceIdentity("Autenticacao de dispositivo invalida");
  });
}

async function bootstrapDeviceAuth(forceFresh) {
  if (bootstrappingAuth) {
    return;
  }
  bootstrappingAuth = true;

  try {
    setState({ bootMessage: "Inicializando dispositivo..." });
    ui.setBootStatus("Inicializando dispositivo...");

    if (forceFresh) {
      clearTokens();
      clearUser();
      setState({ tokens: null, user: null, device: null, wsConnected: false });
      wsClient.disconnect();
    }

    await checkServerHealth();

    if (!forceFresh && state.tokens?.access_token) {
      try {
        const me = await apiRequest("/api/me", { method: "GET" });
        setState({ user: me });
        saveUser(me);
        await registerCurrentDevice();
        await refreshDevices();
        setState({ bootMessage: "Dispositivo autenticado." });
        return;
      } catch {
        clearTokens();
        clearUser();
        setState({ tokens: null, user: null, device: null });
      }
    }

    await deviceLogin();
    await refreshDevices();
    setState({ bootMessage: "Dispositivo autenticado." });
  } catch (error) {
    const message = humanizeNetworkError(error);
    setState({
      tokens: null,
      user: null,
      device: null,
      wsConnected: false,
      bootMessage: `Falha ao autenticar dispositivo: ${message}`
    });
    ui.log(`Falha no bootstrap do dispositivo: ${message}`);
  } finally {
    bootstrappingAuth = false;
  }
}

async function deviceLogin() {
  const deviceId = getDeviceId();
  const deviceAuthKey = getDeviceAuthKey();

  let runtimeInfo = {
    platform: navigator.platform || "unknown",
    app_version: import.meta.env.VITE_APP_VERSION || "0.1.0",
    machine_name: state.settings.deviceName,
    mac_address: ""
  };
  try {
    runtimeInfo = await invoke("get_runtime_info");
  } catch {
    // browser fallback
  }

  const payload = await apiRequest("/api/auth/device-login", {
    method: "POST",
    timeoutMs: 15000,
    retryAttempts: 3,
    retryDelayMs: 2500,
    headers: {
      "X-Device-ID": deviceId,
      "X-Device-Key": deviceAuthKey
    },
    body: JSON.stringify({
      device_id: deviceId,
      device_name: state.settings.deviceName,
      machine_name: runtimeInfo.machine_name || runtimeInfo.hostname || state.settings.deviceName,
      mac_address: runtimeInfo.mac_address || "",
      platform: runtimeInfo.platform || "unknown",
      app_version: runtimeInfo.app_version || import.meta.env.VITE_APP_VERSION || "0.1.0"
    })
  });

  setState({
    tokens: payload,
    user: payload.user,
    device: payload.device
  });
  saveTokens(payload);
  saveUser(payload.user);
  wsClient.disconnect();
  wsClient.connect();
  ui.log(`Dispositivo autenticado: ${payload.device?.id || deviceId}`);
}

async function registerCurrentDevice() {
  if (!state.tokens?.access_token) {
    return;
  }
  await registerCurrentDeviceInternal(true);
}

async function registerCurrentDeviceInternal(allowIdentityReset) {
  const deviceId = getDeviceId();
  const deviceAuthKey = getDeviceAuthKey();
  let runtimeInfo = {
    platform: navigator.platform || "unknown",
    app_version: import.meta.env.VITE_APP_VERSION || "0.1.0",
    machine_name: state.settings.deviceName,
    mac_address: ""
  };
  try {
    runtimeInfo = await invoke("get_runtime_info");
  } catch {
    // fallback browser mode
  }

  try {
    const registered = await apiRequest("/api/devices/register", {
      method: "POST",
      headers: {
        "X-Device-ID": deviceId,
        "X-Device-Key": deviceAuthKey
      },
      body: JSON.stringify({
        device_id: deviceId,
        device_name: state.settings.deviceName,
        machine_name: runtimeInfo.machine_name || runtimeInfo.hostname || state.settings.deviceName,
        mac_address: runtimeInfo.mac_address || "",
        platform: runtimeInfo.platform || "unknown",
        app_version: runtimeInfo.app_version || import.meta.env.VITE_APP_VERSION || "0.1.0"
      })
    });

    setState({ device: registered });
    wsClient.disconnect();
    wsClient.connect();
  } catch (error) {
    if (allowIdentityReset && isRecoverableDeviceRegistrationError(error?.message || "")) {
      resetDeviceIdentity();
      await bootstrapDeviceAuth(true);
      return;
    }
    throw error;
  }
}

async function refreshDevices() {
  if (!state.tokens?.access_token) {
    return;
  }
  try {
    const result = await apiRequest("/api/devices?scope=global", { method: "GET" });
    setState({ devices: result.devices || [] });
    ui.renderDevices(onConnectToDevice);
  } catch (error) {
    ui.log(`Falha ao atualizar dispositivos: ${error.message}`);
  }
}

async function onConnectToDevice(device) {
  return requestConnectionToDevice(device?.id, device?.device_name || "dispositivo");
}

async function onConnectById() {
  const targetId = (ui.elements.partnerIdInput.value || "").trim();
  if (!targetId) {
    ui.log("Informe o ID do parceiro para conectar");
    return;
  }
  if (!/^\d{9}$/.test(targetId)) {
    ui.log("ID invalido. Use exatamente 9 digitos numericos.");
    return;
  }
  await requestConnectionToDevice(targetId, targetId);
}

async function requestConnectionToDevice(targetDeviceID, targetLabel) {
  if (!targetDeviceID) {
    ui.log("ID de dispositivo invalido");
    return;
  }
  try {
    const response = await apiRequest("/api/sessions/request", {
      method: "POST",
      body: JSON.stringify({ target_device_id: targetDeviceID })
    });
    const request = response.session_request;
    ui.log(`Solicitacao enviada para ${targetLabel} (${request.id})`);
  } catch (error) {
    ui.log(`Falha ao solicitar conexao: ${error.message}`);
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
        accept
      })
    });
    ui.log(`Solicitacao ${accept ? "aceita" : "rejeitada"}`);
  } catch (error) {
    ui.log(`Erro ao responder solicitacao: ${error.message}`);
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

async function onEndSession() {
  if (!state.activeSession) {
    ui.log("Nenhuma sessao ativa para encerrar");
    return;
  }

  try {
    await apiRequest("/api/sessions/end", {
      method: "POST",
      body: JSON.stringify({
        session_id: state.activeSession.id,
        session_token: state.activeSession.session_token
      })
    });
    cleanupActiveSession("Sessao encerrada");
  } catch (error) {
    ui.log(`Falha ao encerrar sessao: ${error.message}`);
  }
}

async function onCopyMyId() {
  const deviceID = state.device?.id;
  if (!deviceID) {
    ui.log("ID do dispositivo ainda nao disponivel");
    return;
  }
  try {
    await navigator.clipboard.writeText(deviceID);
    ui.log("ID copiado para area de transferencia");
  } catch {
    ui.log(`Seu ID: ${deviceID}`);
  }
}

async function onReprovisionDevice() {
  ui.log("Reprovisionando dispositivo...");
  resetDeviceIdentity();
  await bootstrapDeviceAuth(true);
}

function onSaveSettings() {
  const previous = state.settings || {};
  const serverUrl = normalizeServerUrl(ui.elements.settingsServerUrl.value.trim());
  const deviceName = ui.elements.settingsDeviceName.value.trim();
  const updated = {
    serverUrl: serverUrl || normalizeServerUrl(previous.serverUrl),
    deviceName: deviceName || previous.deviceName,
    autoStartPrepared: ui.elements.settingsAutoStart.checked
  };
  saveSettings(updated);
  setState({ settings: updated });
  ui.showSettings(false);
  ui.log("Configuracoes salvas");
  checkServerHealth();

  if (!state.tokens?.access_token) {
    bootstrapDeviceAuth(false);
  }
}

function cleanupActiveSession(message) {
  stopHostSharing(ui.log);
  unbindControllerInput();
  setState({
    activeSession: null,
    incomingFiles: []
  });
  ui.log(message);
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

async function recoverDeviceIdentity(trigger) {
  if (recoveringDeviceIdentity) {
    return;
  }
  const now = Date.now();
  if (now - lastDeviceRecoveryAt < 20000) {
    return;
  }
  lastDeviceRecoveryAt = now;
  recoveringDeviceIdentity = true;

  try {
    ui.log(`${trigger}. Renovando identidade deste dispositivo...`);
    resetDeviceIdentity();
    await bootstrapDeviceAuth(true);
    await refreshDevices();
    ui.log("Identidade do dispositivo renovada com sucesso.");
  } catch (error) {
    ui.log(`Falha ao renovar identidade do dispositivo: ${error.message || error}`);
  } finally {
    recoveringDeviceIdentity = false;
  }
}

function isRecoverableDeviceRegistrationError(message) {
  const lower = String(message || "").toLowerCase();
  return (
    lower.includes("device already belongs to another user") ||
    lower.includes("device identity mismatch") ||
    lower.includes("device authentication failed") ||
    lower.includes("invalid device authentication")
  );
}

function humanizeNetworkError(error) {
  const message = error?.message || String(error);
  if (message.includes("Failed to fetch") || message.includes("NetworkError") || message.includes("abort")) {
    return "Servidor indisponivel ou iniciando (Render free pode levar ate ~1 min). Tente novamente.";
  }
  return message;
}

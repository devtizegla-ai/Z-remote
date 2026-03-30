import { state } from "./state.js";

export function createUI() {
  const logs = [];

  const elements = {
    authSection: document.getElementById("authSection"),
    appSection: document.getElementById("appSection"),
    authSettingsBtn: document.getElementById("authSettingsBtn"),
    retryBootstrapBtn: document.getElementById("retryBootstrapBtn"),
    bootStatus: document.getElementById("bootStatus"),
    devicesList: document.getElementById("devicesList"),
    connectionBadge: document.getElementById("connectionBadge"),
    userLabel: document.getElementById("userLabel"),
    deviceLabel: document.getElementById("deviceLabel"),
    myDeviceId: document.getElementById("myDeviceId"),
    copyDeviceIdBtn: document.getElementById("copyDeviceIdBtn"),
    partnerIdInput: document.getElementById("partnerIdInput"),
    connectByIdBtn: document.getElementById("connectByIdBtn"),
    logsList: document.getElementById("logsList"),
    sessionCard: document.getElementById("sessionCard"),
    sessionMeta: document.getElementById("sessionMeta"),
    remoteFrame: document.getElementById("remoteFrame"),
    requestModal: document.getElementById("requestModal"),
    requestModalText: document.getElementById("requestModalText"),
    incomingFiles: document.getElementById("incomingFiles"),
    settingsModal: document.getElementById("settingsModal"),
    settingsServerUrl: document.getElementById("settingsServerUrl"),
    settingsDeviceName: document.getElementById("settingsDeviceName"),
    settingsAutoStart: document.getElementById("settingsAutoStart"),
    fileProgress: document.getElementById("fileProgress"),
    fileProgressLabel: document.getElementById("fileProgressLabel"),
    refreshDevicesBtn: document.getElementById("refreshDevicesBtn"),
    openSettingsBtn: document.getElementById("openSettingsBtn"),
    closeSettingsBtn: document.getElementById("closeSettingsBtn"),
    saveSettingsBtn: document.getElementById("saveSettingsBtn"),
    logoutBtn: document.getElementById("logoutBtn"),
    startShareBtn: document.getElementById("startShareBtn"),
    stopShareBtn: document.getElementById("stopShareBtn"),
    endSessionBtn: document.getElementById("endSessionBtn"),
    fileInput: document.getElementById("fileInput"),
    sendFileBtn: document.getElementById("sendFileBtn"),
    acceptRequestBtn: document.getElementById("acceptRequestBtn"),
    rejectRequestBtn: document.getElementById("rejectRequestBtn")
  };

  function log(message) {
    const timestamp = new Date().toLocaleTimeString();
    logs.unshift(`[${timestamp}] ${message}`);
    logs.splice(60);
    renderLogs();
  }

  function renderLogs() {
    elements.logsList.innerHTML = "";
    logs.forEach((item) => {
      const node = document.createElement("div");
      node.className = "log-item";
      node.textContent = item;
      elements.logsList.appendChild(node);
    });
  }

  function renderDevices(onConnect) {
    elements.devicesList.innerHTML = "";
    if (!state.devices.length) {
      const empty = document.createElement("p");
      empty.className = "muted";
      empty.textContent = "Nenhum dispositivo online.";
      elements.devicesList.appendChild(empty);
      return;
    }

    state.devices.forEach((device) => {
      const card = document.createElement("div");
      card.className = "device-card";
      const isSelf = state.device && device.id === state.device.id;

      const info = document.createElement("div");
      const title = document.createElement("strong");
      const ownerPrefix = device.owner_name ? `${device.owner_name} - ` : "";
      title.textContent = `${ownerPrefix}${device.device_name}`;
      const details = document.createElement("p");
      details.className = "muted";
      details.textContent = `${device.machine_name || "host"} - ${device.platform} - ${device.status} - ID: ${device.id}`;
      info.appendChild(title);
      info.appendChild(details);
      card.appendChild(info);

      const button = document.createElement("button");
      button.className = "btn";
      button.textContent = isSelf ? "Este dispositivo" : "Conectar";
      button.disabled = isSelf;
      button.addEventListener("click", () => onConnect(device));
      card.appendChild(button);
      elements.devicesList.appendChild(card);
    });
  }

  function renderIncomingFiles(onDownload) {
    elements.incomingFiles.innerHTML = "";
    for (const item of state.incomingFiles) {
      const row = document.createElement("div");
      row.className = "file-row";
      const fileText = document.createElement("span");
      fileText.textContent = `${item.filename} (${Math.round(item.size_bytes / 1024)} KB)`;
      row.appendChild(fileText);

      const button = document.createElement("button");
      button.className = "btn";
      button.textContent = "Baixar";
      button.addEventListener("click", () => onDownload(item.id));
      row.appendChild(button);
      elements.incomingFiles.appendChild(row);
    }
  }

  function render() {
    const logged = Boolean(state.tokens?.access_token && state.user && state.device);
    elements.authSection.classList.toggle("hidden", logged);
    elements.appSection.classList.toggle("hidden", !logged);

    if (logged) {
      const serverReady = state.serverReachable === true;
      const online = state.wsConnected || serverReady;
      elements.connectionBadge.textContent = state.wsConnected
        ? "Sessao Online"
        : serverReady
          ? "Servidor OK (WS reconectando)"
          : "Sessao Offline";
      elements.connectionBadge.classList.toggle("connected", online);
      elements.connectionBadge.classList.toggle("disconnected", !online);
    } else {
      const serverReady = state.serverReachable === true;
      elements.connectionBadge.textContent = serverReady ? "Servidor OK" : "Servidor Indisponivel";
      elements.connectionBadge.classList.toggle("connected", serverReady);
      elements.connectionBadge.classList.toggle("disconnected", !serverReady);
      elements.bootStatus.textContent = state.bootMessage || "Inicializando dispositivo...";
    }

    elements.userLabel.textContent = state.user?.name || "Dispositivo";
    elements.deviceLabel.textContent = state.device?.device_name || "Sem registro";
    elements.myDeviceId.textContent = state.device?.id || "-";

    const hasSession = Boolean(state.activeSession);
    elements.sessionCard.classList.toggle("hidden", !hasSession);
    if (hasSession) {
      const role = state.activeSession.requester_device_id === state.device.id ? "controller" : "host";
      elements.sessionMeta.textContent = `Sessao ${state.activeSession.id} - Perfil: ${role}`;
    } else {
      elements.remoteFrame.removeAttribute("src");
      elements.incomingFiles.innerHTML = "";
    }

    if (state.pendingRequest) {
      elements.requestModal.classList.remove("hidden");
      elements.requestModalText.textContent = `Dispositivo ${state.pendingRequest.requester_device_id} quer iniciar conexao.`;
    } else {
      elements.requestModal.classList.add("hidden");
    }

    elements.settingsServerUrl.value = state.settings?.serverUrl || "";
    elements.settingsDeviceName.value = state.settings?.deviceName || "";
    elements.settingsAutoStart.checked = Boolean(state.settings?.autoStartPrepared);
  }

  function setRemoteFrame(dataUrl) {
    elements.remoteFrame.src = dataUrl;
  }

  function showSettings(open) {
    elements.settingsModal.classList.toggle("hidden", !open);
  }

  function updateUploadProgress(value) {
    elements.fileProgress.value = value;
    elements.fileProgressLabel.textContent = `${value}%`;
  }

  function setBootStatus(message) {
    elements.bootStatus.textContent = message;
  }

  return {
    elements,
    log,
    render,
    renderDevices,
    renderIncomingFiles,
    setRemoteFrame,
    showSettings,
    updateUploadProgress,
    setBootStatus
  };
}

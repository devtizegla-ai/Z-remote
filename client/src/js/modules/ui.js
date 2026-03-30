import { state } from "./state.js";

export function createUI() {
  const logs = [];

  const elements = {
    authSection: document.getElementById("authSection"),
    appSection: document.getElementById("appSection"),
    tabLogin: document.getElementById("tabLogin"),
    tabRegister: document.getElementById("tabRegister"),
    loginForm: document.getElementById("loginForm"),
    loginEmail: document.getElementById("loginEmail"),
    loginPassword: document.getElementById("loginPassword"),
    registerForm: document.getElementById("registerForm"),
    registerName: document.getElementById("registerName"),
    registerEmail: document.getElementById("registerEmail"),
    registerPassword: document.getElementById("registerPassword"),
    devicesList: document.getElementById("devicesList"),
    connectionBadge: document.getElementById("connectionBadge"),
    userLabel: document.getElementById("userLabel"),
    deviceLabel: document.getElementById("deviceLabel"),
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
    fileInput: document.getElementById("fileInput"),
    sendFileBtn: document.getElementById("sendFileBtn"),
    acceptRequestBtn: document.getElementById("acceptRequestBtn"),
    rejectRequestBtn: document.getElementById("rejectRequestBtn")
  };

  function showTab(tab) {
    const loginActive = tab === "login";
    elements.tabLogin.classList.toggle("active", loginActive);
    elements.tabRegister.classList.toggle("active", !loginActive);
    elements.loginForm.classList.toggle("active", loginActive);
    elements.registerForm.classList.toggle("active", !loginActive);
  }

  function log(message) {
    const timestamp = new Date().toLocaleTimeString();
    logs.unshift(`[${timestamp}] ${message}`);
    logs.splice(50);
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
      elements.devicesList.innerHTML = `<p class="muted">Nenhum dispositivo online.</p>`;
      return;
    }

    state.devices.forEach((device) => {
      const card = document.createElement("div");
      card.className = "device-card";
      const isSelf = state.device && device.id === state.device.id;
      card.innerHTML = `
        <div>
          <strong>${device.device_name}</strong>
          <p class="muted">${device.platform} · ${device.status}</p>
        </div>
      `;
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
      row.innerHTML = `<span>${item.filename} (${Math.round(item.size_bytes / 1024)} KB)</span>`;
      const button = document.createElement("button");
      button.className = "btn";
      button.textContent = "Baixar";
      button.addEventListener("click", () => onDownload(item.id));
      row.appendChild(button);
      elements.incomingFiles.appendChild(row);
    }
  }

  function render() {
    const logged = Boolean(state.tokens?.access_token && state.user);
    elements.authSection.classList.toggle("hidden", logged);
    elements.appSection.classList.toggle("hidden", !logged);

    elements.connectionBadge.textContent = state.wsConnected ? "Online" : "Offline";
    elements.connectionBadge.classList.toggle("connected", state.wsConnected);
    elements.connectionBadge.classList.toggle("disconnected", !state.wsConnected);

    elements.userLabel.textContent = state.user?.name || "Usuário";
    elements.deviceLabel.textContent = state.device?.device_name || "Dispositivo";

    const hasSession = Boolean(state.activeSession);
    elements.sessionCard.classList.toggle("hidden", !hasSession);
    if (hasSession) {
      const role = state.activeSession.requester_device_id === state.device.id ? "controller" : "host";
      elements.sessionMeta.textContent = `Sessão ${state.activeSession.id} · Perfil: ${role}`;
    } else {
      elements.remoteFrame.removeAttribute("src");
      elements.incomingFiles.innerHTML = "";
    }

    if (state.pendingRequest) {
      elements.requestModal.classList.remove("hidden");
      elements.requestModalText.textContent = `Dispositivo ${state.pendingRequest.requester_device_id} quer iniciar conexão.`;
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

  return {
    elements,
    showTab,
    log,
    render,
    renderDevices,
    renderIncomingFiles,
    setRemoteFrame,
    showSettings,
    updateUploadProgress
  };
}


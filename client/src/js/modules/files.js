import { getDeviceAuthKey, getDeviceId } from "./config.js";
import { state } from "./state.js";

export function uploadSessionFile(file, onProgress) {
  return new Promise((resolve, reject) => {
    if (!state.activeSession) {
      reject(new Error("Nenhuma sessão ativa"));
      return;
    }

    const peerDeviceId =
      state.activeSession.requester_device_id === state.device.id
        ? state.activeSession.target_device_id
        : state.activeSession.requester_device_id;

    const form = new FormData();
    form.append("session_id", state.activeSession.id);
    form.append("session_token", state.activeSession.session_token);
    form.append("from_device_id", state.device.id);
    form.append("to_device_id", peerDeviceId);
    form.append("file", file);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${state.settings.serverUrl}/api/files/upload`);
    xhr.setRequestHeader("Authorization", `Bearer ${state.tokens.access_token}`);
    xhr.setRequestHeader("X-Device-ID", getDeviceId());
    xhr.setRequestHeader("X-Device-Key", getDeviceAuthKey());

    xhr.upload.addEventListener("progress", (event) => {
      if (!event.lengthComputable) {
        return;
      }
      const pct = Math.round((event.loaded / event.total) * 100);
      onProgress(pct);
    });

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        try {
          const parsed = JSON.parse(xhr.responseText);
          reject(new Error(parsed.error || "Falha no upload"));
        } catch {
          reject(new Error(`Falha no upload (${xhr.status})`));
        }
      }
    };

    xhr.onerror = () => reject(new Error("Falha de rede no upload"));
    xhr.send(form);
  });
}

export async function downloadTransfer(transferId) {
  const response = await fetch(
    `${state.settings.serverUrl}/api/files/download?transfer_id=${encodeURIComponent(transferId)}`,
    {
      headers: {
        Authorization: `Bearer ${state.tokens.access_token}`,
        "X-Device-ID": getDeviceId(),
        "X-Device-Key": getDeviceAuthKey()
      }
    }
  );

  if (!response.ok) {
    let message = `Erro no download (${response.status})`;
    try {
      const payload = await response.json();
      message = payload.error || message;
    } catch {
      // ignore parse errors
    }
    throw new Error(message);
  }

  const blob = await response.blob();
  const disposition = response.headers.get("Content-Disposition") || "";
  const fileNameMatch = disposition.match(/filename=\"(.+)\"/);
  const fileName = fileNameMatch?.[1] || `transfer_${transferId}`;

  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}


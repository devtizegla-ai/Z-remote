import { getDeviceAuthKey, getDeviceId, normalizeServerUrl } from "./config.js";
import { state } from "./state.js";

const UPLOAD_ATTEMPTS = 3;
const DOWNLOAD_ATTEMPTS = 3;

export async function uploadSessionFile(file, onProgress) {
  if (!state.activeSession) {
    throw new Error("Nenhuma sessao ativa");
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

  for (let attempt = 1; attempt <= UPLOAD_ATTEMPTS; attempt++) {
    try {
      return await uploadOnce(form, onProgress);
    } catch (error) {
      const retryable = Boolean(error?.retryable);
      if (!retryable || attempt === UPLOAD_ATTEMPTS) {
        throw error?.cause || error;
      }
      await delay(900 * attempt);
    }
  }

  throw new Error("Falha no upload");
}

export async function downloadTransfer(transferId) {
  const url = `${buildApiUrl("/api/files/download")}?transfer_id=${encodeURIComponent(transferId)}`;

  let response = null;
  for (let attempt = 1; attempt <= DOWNLOAD_ATTEMPTS; attempt++) {
    try {
      response = await fetchWithTimeout(
        url,
        {
          headers: {
            Authorization: `Bearer ${state.tokens.access_token}`,
            "X-Device-ID": getDeviceId(),
            "X-Device-Key": getDeviceAuthKey()
          }
        },
        45000
      );
    } catch {
      if (attempt === DOWNLOAD_ATTEMPTS) {
        throw new Error("Falha de rede no download");
      }
      await delay(900 * attempt);
      continue;
    }

    if (response.ok) {
      break;
    }

    if (!isRetryableStatus(response.status) || attempt === DOWNLOAD_ATTEMPTS) {
      let message = `Erro no download (${response.status})`;
      try {
        const payload = await response.json();
        message = payload.error || message;
      } catch {
        // ignore parse errors
      }
      throw new Error(message);
    }

    await delay(900 * attempt);
  }

  if (!response || !response.ok) {
    throw new Error("Falha no download");
  }

  const blob = await response.blob();
  const disposition = response.headers.get("Content-Disposition") || "";
  const fileName = parseDownloadFilename(disposition) || `transfer_${transferId}`;

  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(objectUrl);
}

function uploadOnce(form, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", buildApiUrl("/api/files/upload"));
    xhr.timeout = 45000;
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
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch {
          resolve({});
        }
        return;
      }

      let message = `Falha no upload (${xhr.status})`;
      try {
        const parsed = JSON.parse(xhr.responseText || "{}");
        message = parsed.error || message;
      } catch {
        // ignore parse errors
      }

      reject({
        retryable: isRetryableStatus(xhr.status),
        cause: new Error(message)
      });
    };

    xhr.onerror = () => reject({ retryable: true, cause: new Error("Falha de rede no upload") });
    xhr.ontimeout = () => reject({ retryable: true, cause: new Error("Timeout no upload") });
    xhr.onabort = () => reject({ retryable: true, cause: new Error("Upload interrompido") });
    xhr.send(form);
  });
}

function parseDownloadFilename(disposition) {
  const utf8 = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8?.[1]) {
    return decodeURIComponent(utf8[1]);
  }
  const simple = disposition.match(/filename="(.+?)"/i);
  return simple?.[1] || null;
}

function buildApiUrl(path) {
  const base = normalizeServerUrl(state.settings.serverUrl);
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return `${base}${normalizedPath}`;
}

function isRetryableStatus(status) {
  return status >= 500 || status === 429;
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function fetchWithTimeout(url, options, timeoutMs) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    clearTimeout(timeout);
  }
}

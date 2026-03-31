import { invoke } from "@tauri-apps/api/core";
import { recoverInvalidDeviceAuth } from "./api.js";
import { getDeviceAuthKey, getDeviceId, normalizeServerUrl } from "./config.js";
import { state } from "./state.js";

const UPLOAD_ATTEMPTS = 3;
const DOWNLOAD_ATTEMPTS = 3;
const FILE_UPLOAD_PATH = "/api/files/upload";
const FILE_DOWNLOAD_PATH = "/api/files/download";

export async function uploadSessionFile(file, onProgress, options = {}) {
  const session = state.activeSession;
  if (!session) {
    throw new Error("Nenhuma sessao ativa");
  }
  if (!state.device?.id) {
    throw new Error("Identidade de dispositivo indisponivel");
  }

  const peerDeviceId =
    session.requester_device_id === state.device.id ? session.target_device_id : session.requester_device_id;
  const uploadContext = {
    sessionId: session.id,
    sessionToken: session.session_token,
    fromDeviceId: state.device.id,
    toDeviceId: peerDeviceId
  };

  const safeProgress = typeof onProgress === "function" ? onProgress : () => {};
  let invalidDeviceAuthRetried = false;

  for (let attempt = 1; attempt <= UPLOAD_ATTEMPTS; attempt++) {
    try {
      const form = buildUploadForm(file, uploadContext, options.targetSavePath);
      return await uploadOnce(form, safeProgress);
    } catch (error) {
      const cause = error?.cause || error;
      const recovered = await recoverInvalidDeviceAuth(cause?.message || String(cause || ""), FILE_UPLOAD_PATH, {
        __deviceAuthRetried: invalidDeviceAuthRetried
      });
      if (recovered) {
        invalidDeviceAuthRetried = true;
        continue;
      }

      const retryable = Boolean(error?.retryable);
      if (!retryable || attempt === UPLOAD_ATTEMPTS) {
        throw cause;
      }
      await delay(900 * attempt);
    }
  }

  throw new Error("Falha no upload");
}

export async function downloadTransfer(transferId, options = {}) {
  const url = `${buildApiUrl(FILE_DOWNLOAD_PATH)}?transfer_id=${encodeURIComponent(transferId)}`;
  let invalidDeviceAuthRetried = false;

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

    let message = `Erro no download (${response.status})`;
    try {
      const payload = await response.json();
      message = payload.error || message;
    } catch {
      // ignore parse errors
    }

    const recovered = await recoverInvalidDeviceAuth(message, FILE_DOWNLOAD_PATH, {
      __deviceAuthRetried: invalidDeviceAuthRetried
    });
    if (recovered) {
      invalidDeviceAuthRetried = true;
      continue;
    }

    if (!isRetryableStatus(response.status) || attempt === DOWNLOAD_ATTEMPTS) {
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

  const base64 = await blobToBase64WithoutPrefix(blob);

  if (options.destinationDir) {
    try {
      const savedPath = await invoke("save_download_file_to_path", {
        fileName,
        dataBase64: base64,
        destinationDir: options.destinationDir
      });
      return { savedPath: savedPath || null };
    } catch (error) {
      if (options.requireDestinationSave) {
        throw new Error(`Falha ao salvar no destino remoto: ${error?.message || error}`);
      }
    }
  }

  try {
    const savedPath = await invoke("save_download_file", {
      fileName,
      dataBase64: base64
    });
    return { savedPath: savedPath || null };
  } catch {
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = fileName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(objectUrl);
    return { savedPath: null };
  }
}

function uploadOnce(form, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", buildApiUrl(FILE_UPLOAD_PATH));
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

function buildUploadForm(file, uploadContext, targetSavePath) {
  const form = new FormData();
  form.append("session_id", uploadContext.sessionId);
  form.append("session_token", uploadContext.sessionToken);
  form.append("from_device_id", uploadContext.fromDeviceId);
  form.append("to_device_id", uploadContext.toDeviceId);
  form.append("target_save_path", String(targetSavePath || "").trim());
  form.append("file", file);
  return form;
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

async function blobToBase64WithoutPrefix(blob) {
  const dataUrl = await new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error("Falha ao ler arquivo"));
    reader.readAsDataURL(blob);
  });
  const separatorIndex = dataUrl.indexOf(",");
  return separatorIndex >= 0 ? dataUrl.slice(separatorIndex + 1) : dataUrl;
}

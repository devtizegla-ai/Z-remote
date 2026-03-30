import { getDeviceAuthKey, getDeviceId, normalizeServerUrl } from "./config.js";
import { state } from "./state.js";

let invalidDeviceAuthHandler = null;
let invalidDeviceAuthRecoveryInFlight = false;

export function setInvalidDeviceAuthHandler(handler) {
  invalidDeviceAuthHandler = typeof handler === "function" ? handler : null;
}

export async function apiRequest(path, options = {}) {
  const baseUrl = normalizeServerUrl(state.settings.serverUrl);
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  const url = `${baseUrl}${normalizedPath}`;
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {})
  };

  if (state.tokens?.access_token) {
    headers.Authorization = `Bearer ${state.tokens.access_token}`;
    headers["X-Device-ID"] = getDeviceId();
    headers["X-Device-Key"] = getDeviceAuthKey();
  }

  const response = await fetchWithRetry(url, {
    ...options,
    headers
  });

  if (!response.ok) {
    let message = `Erro HTTP ${response.status}`;
    try {
      const data = await response.json();
      message = data.error || message;
    } catch {
      // ignore body parse errors
    }

    if (
      shouldRecoverFromInvalidDeviceAuth(message, normalizedPath, options) &&
      invalidDeviceAuthHandler
    ) {
      if (!invalidDeviceAuthRecoveryInFlight) {
        invalidDeviceAuthRecoveryInFlight = true;
        try {
          await invalidDeviceAuthHandler(message);
        } finally {
          invalidDeviceAuthRecoveryInFlight = false;
        }
      }
      return apiRequest(path, {
        ...options,
        __deviceAuthRetried: true
      });
    }

    throw new Error(message);
  }

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get("Content-Type") || "";
  if (contentType.includes("application/json")) {
    return response.json();
  }

  return response.blob();
}

async function fetchWithRetry(url, options) {
  const timeoutMs = options.timeoutMs || 45000;
  const retryEnabled = options.retry !== false;
  const retryAttempts = retryEnabled ? (options.retryAttempts || 2) : 1;
  const retryDelayMs = options.retryDelayMs || 3500;

  async function runOnce() {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);
    try {
      return await fetch(url, { ...options, signal: controller.signal });
    } finally {
      clearTimeout(timeout);
    }
  }

  let lastError;
  for (let attempt = 1; attempt <= retryAttempts; attempt++) {
    try {
      return await runOnce();
    } catch (error) {
      lastError = error;
      if (attempt < retryAttempts) {
        await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
      }
    }
  }
  throw lastError;
}

function shouldRecoverFromInvalidDeviceAuth(message, path, options) {
  const lower = (message || "").toLowerCase();
  if (!lower.includes("invalid device authentication")) {
    return false;
  }
  if (options.__deviceAuthRetried) {
    return false;
  }
  if (path === "/api/devices/register" || path === "/health") {
    return false;
  }
  return true;
}


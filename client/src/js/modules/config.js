const SETTINGS_KEY = "ra_mvp_settings";
const TOKENS_KEY = "ra_mvp_tokens";
const USER_KEY = "ra_mvp_user";
const DEVICE_KEY = "ra_mvp_device_id";
const DEVICE_AUTH_KEY = "ra_mvp_device_auth_key";

function generateUUIDCompat() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 14)}`;
}

export function normalizeServerUrl(value) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }

  let normalized = raw;
  if (!/^https?:\/\//i.test(normalized)) {
    if (
      normalized.startsWith("localhost") ||
      normalized.startsWith("127.0.0.1") ||
      normalized.startsWith("0.0.0.0")
    ) {
      normalized = `http://${normalized}`;
    } else {
      normalized = `https://${normalized}`;
    }
  }

  return normalized.replace(/\/+$/, "");
}

export function getDefaultSettings() {
  return {
    serverUrl: normalizeServerUrl(import.meta.env.VITE_SERVER_URL || "http://localhost:8080"),
    deviceName: `Device-${Math.random().toString(36).slice(2, 6)}`,
    autoStartPrepared: false
  };
}

export function loadSettings() {
  const defaults = getDefaultSettings();
  try {
    const parsed = JSON.parse(localStorage.getItem(SETTINGS_KEY) || "{}");
    return {
      ...defaults,
      ...parsed,
      serverUrl: normalizeServerUrl(parsed.serverUrl || defaults.serverUrl)
    };
  } catch {
    return defaults;
  }
}

export function saveSettings(settings) {
  const normalized = {
    ...settings,
    serverUrl: normalizeServerUrl(settings.serverUrl)
  };
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(normalized));
}

export function loadTokens() {
  try {
    return JSON.parse(localStorage.getItem(TOKENS_KEY) || "null");
  } catch {
    return null;
  }
}

export function saveTokens(tokens) {
  localStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
}

export function clearTokens() {
  localStorage.removeItem(TOKENS_KEY);
}

export function loadUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || "null");
  } catch {
    return null;
  }
}

export function saveUser(user) {
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearUser() {
  localStorage.removeItem(USER_KEY);
}

export function getDeviceId() {
  const existing = localStorage.getItem(DEVICE_KEY);
  if (existing) {
    return existing;
  }
  const newId = `dev_local_${generateUUIDCompat()}`;
  localStorage.setItem(DEVICE_KEY, newId);
  return newId;
}

export function getDeviceAuthKey() {
  const existing = localStorage.getItem(DEVICE_AUTH_KEY);
  if (existing) {
    return existing;
  }
  const randomChunk = `${generateUUIDCompat()}${generateUUIDCompat().replaceAll("-", "")}`;
  const newKey = `dkey_${randomChunk}`;
  localStorage.setItem(DEVICE_AUTH_KEY, newKey);
  return newKey;
}

export function resetDeviceIdentity() {
  localStorage.removeItem(DEVICE_KEY);
  localStorage.removeItem(DEVICE_AUTH_KEY);
  return {
    deviceId: getDeviceId(),
    deviceAuthKey: getDeviceAuthKey()
  };
}


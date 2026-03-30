const SETTINGS_KEY = "ra_mvp_settings";
const TOKENS_KEY = "ra_mvp_tokens";
const USER_KEY = "ra_mvp_user";
const DEVICE_KEY = "ra_mvp_device_id";

export function getDefaultSettings() {
  return {
    serverUrl: import.meta.env.VITE_SERVER_URL || "http://localhost:8080",
    deviceName: `Device-${Math.random().toString(36).slice(2, 6)}`,
    autoStartPrepared: false
  };
}

export function loadSettings() {
  const defaults = getDefaultSettings();
  try {
    const parsed = JSON.parse(localStorage.getItem(SETTINGS_KEY) || "{}");
    return { ...defaults, ...parsed };
  } catch {
    return defaults;
  }
}

export function saveSettings(settings) {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
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
  const newId = `dev_local_${crypto.randomUUID()}`;
  localStorage.setItem(DEVICE_KEY, newId);
  return newId;
}


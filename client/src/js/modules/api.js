import { state } from "./state.js";

export async function apiRequest(path, options = {}) {
  const url = `${state.settings.serverUrl}${path}`;
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {})
  };

  if (state.tokens?.access_token) {
    headers.Authorization = `Bearer ${state.tokens.access_token}`;
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


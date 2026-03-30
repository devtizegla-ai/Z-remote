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

  const response = await fetchWithTimeout(url, {
    ...options,
    headers
  }, options.timeoutMs || 45000);

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

async function fetchWithTimeout(url, options, timeoutMs) {
  async function runOnce() {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);
    try {
      return await fetch(url, { ...options, signal: controller.signal });
    } finally {
      clearTimeout(timeout);
    }
  }

  try {
    return await runOnce();
  } catch (error) {
    if (options.retry === false) {
      throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, 3500));
    return runOnce();
  }
}


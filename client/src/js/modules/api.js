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

  const response = await fetch(url, {
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


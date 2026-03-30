const listeners = new Set();

export const state = {
  settings: null,
  tokens: null,
  user: null,
  device: null,
  devices: [],
  wsConnected: false,
  pendingRequest: null,
  activeSession: null,
  incomingFiles: []
};

export function subscribe(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function setState(patch) {
  Object.assign(state, patch);
  for (const listener of listeners) {
    listener(state);
  }
}


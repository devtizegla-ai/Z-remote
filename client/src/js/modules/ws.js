import { getDeviceAuthKey } from "./config.js";
import { state, setState } from "./state.js";

class WSClient {
  constructor() {
    this.socket = null;
    this.handlers = new Map();
    this.autoReconnect = true;
  }

  on(type, handler) {
    const handlers = this.handlers.get(type) || [];
    handlers.push(handler);
    this.handlers.set(type, handlers);
  }

  emit(type, payload) {
    for (const handler of this.handlers.get(type) || []) {
      handler(payload);
    }
  }

  connect() {
    if (!state.tokens?.access_token || !state.device?.id) {
      return;
    }
    if (this.socket && (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)) {
      return;
    }

    this.autoReconnect = true;

    const wsBase = state.settings.serverUrl.replace("http://", "ws://").replace("https://", "wss://");
    const url = `${wsBase}/ws?device_id=${encodeURIComponent(state.device.id)}`;
    const protocols = [`access.${state.tokens.access_token}`, `dkey.${getDeviceAuthKey()}`];
    const socket = new WebSocket(url, protocols);
    this.socket = socket;

    socket.onopen = () => {
      if (this.socket !== socket) {
        return;
      }
      setState({ wsConnected: true });
      this.emit("open", null);
    };

    socket.onclose = () => {
      if (this.socket !== socket) {
        return;
      }
      this.socket = null;
      setState({ wsConnected: false });
      this.emit("close", null);
      setTimeout(() => {
        if (this.autoReconnect && !this.socket && state.tokens?.access_token) {
          this.connect();
        }
      }, 2500);
    };

    socket.onerror = () => {
      this.emit("error", new Error("Falha no WebSocket"));
    };

    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.emit("message", message);
        if (message.type) {
          this.emit(message.type, message);
        }
      } catch {
        this.emit("error", new Error("Mensagem WS inválida"));
      }
    };
  }

  disconnect() {
    if (this.socket) {
      this.autoReconnect = false;
      this.socket.close();
      this.socket = null;
    }
  }

  send(type, payload = {}) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return;
    }
    this.socket.send(JSON.stringify({ type, ...payload }));
  }

  sendSessionSignal(kind, payload = {}) {
    if (!state.activeSession) {
      return;
    }
    this.send("session_signal", {
      session_id: state.activeSession.id,
      session_token: state.activeSession.session_token,
      kind,
      payload
    });
  }
}

export const wsClient = new WSClient();


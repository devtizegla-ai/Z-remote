import { state, setState } from "./state.js";

class WSClient {
  constructor() {
    this.socket = null;
    this.handlers = new Map();
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

    const wsBase = state.settings.serverUrl.replace("http://", "ws://").replace("https://", "wss://");
    const url = `${wsBase}/ws?token=${encodeURIComponent(state.tokens.access_token)}&device_id=${encodeURIComponent(state.device.id)}`;
    this.socket = new WebSocket(url);

    this.socket.onopen = () => {
      setState({ wsConnected: true });
      this.emit("open", null);
    };

    this.socket.onclose = () => {
      setState({ wsConnected: false });
      this.emit("close", null);
      setTimeout(() => {
        if (state.tokens?.access_token) {
          this.connect();
        }
      }, 2500);
    };

    this.socket.onerror = () => {
      this.emit("error", new Error("Falha no WebSocket"));
    };

    this.socket.onmessage = (event) => {
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


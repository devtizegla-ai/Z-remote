PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  device_name TEXT NOT NULL,
  platform TEXT NOT NULL,
  app_version TEXT NOT NULL,
  status TEXT NOT NULL,
  last_seen_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS session_requests (
  id TEXT PRIMARY KEY,
  requester_device_id TEXT NOT NULL,
  target_device_id TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  responded_at DATETIME,
  FOREIGN KEY(requester_device_id) REFERENCES devices(id) ON DELETE CASCADE,
  FOREIGN KEY(target_device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS remote_sessions (
  id TEXT PRIMARY KEY,
  requester_device_id TEXT NOT NULL,
  target_device_id TEXT NOT NULL,
  session_token TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at DATETIME,
  ended_at DATETIME,
  token_expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(requester_device_id) REFERENCES devices(id) ON DELETE CASCADE,
  FOREIGN KEY(target_device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS file_transfers (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  status TEXT NOT NULL,
  storage_path TEXT NOT NULL,
  uploaded_by_device_id TEXT NOT NULL,
  target_device_id TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(session_id) REFERENCES remote_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY(uploaded_by_device_id) REFERENCES devices(id) ON DELETE CASCADE,
  FOREIGN KEY(target_device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  device_id TEXT,
  action TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_devices_user_status ON devices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_session_requests_target_status ON session_requests(target_device_id, status);
CREATE INDEX IF NOT EXISTS idx_remote_sessions_status ON remote_sessions(status);
CREATE INDEX IF NOT EXISTS idx_file_transfers_session ON file_transfers(session_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_created ON audit_logs(user_id, created_at);


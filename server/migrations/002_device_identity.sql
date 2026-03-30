ALTER TABLE devices ADD COLUMN machine_name TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN mac_address TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN device_key_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_devices_user_machine ON devices(user_id, machine_name);

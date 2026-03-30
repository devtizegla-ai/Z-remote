use base64::{engine::general_purpose, Engine as _};
use enigo::{Axis, Button, Coordinate, Direction, Enigo, Key, Keyboard, Mouse, Settings};
use image::{codecs::jpeg::JpegEncoder, DynamicImage};
use screenshots::Screen;
use serde::Serialize;
use serde_json::Value;
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Serialize)]
struct RuntimeInfo {
    platform: String,
    arch: String,
    app_version: String,
    machine_name: String,
    mac_address: String,
}

#[tauri::command]
fn get_runtime_info(app: tauri::AppHandle) -> RuntimeInfo {
    let machine_name = hostname::get()
        .ok()
        .and_then(|name| name.into_string().ok())
        .unwrap_or_else(|| "unknown".to_string());
    let mac_address = mac_address::get_mac_address()
        .ok()
        .flatten()
        .map(|value| value.to_string())
        .unwrap_or_default();

    RuntimeInfo {
        platform: std::env::consts::OS.to_string(),
        arch: std::env::consts::ARCH.to_string(),
        app_version: app.package_info().version.to_string(),
        machine_name,
        mac_address,
    }
}

#[tauri::command]
fn capture_primary_screen_jpeg(quality: Option<u8>) -> Result<String, String> {
    let screens = Screen::all().map_err(|e| format!("failed to enumerate screens: {e}"))?;
    let screen = screens
        .first()
        .ok_or_else(|| "no screen available".to_string())?;

    let image = screen
        .capture()
        .map_err(|e| format!("failed to capture screen: {e}"))?;

    let q = quality.unwrap_or(60).clamp(35, 90);
    let mut jpeg_bytes = Vec::new();
    {
        let dynamic = DynamicImage::ImageRgba8(image);
        let mut encoder = JpegEncoder::new_with_quality(&mut jpeg_bytes, q);
        encoder
            .encode_image(&dynamic)
            .map_err(|e| format!("failed to encode jpeg: {e}"))?;
    }

    let encoded = general_purpose::STANDARD.encode(jpeg_bytes);
    Ok(format!("data:image/jpeg;base64,{encoded}"))
}

#[tauri::command]
fn save_download_file(file_name: String, data_base64: String) -> Result<Option<String>, String> {
    let bytes = general_purpose::STANDARD
        .decode(data_base64)
        .map_err(|e| format!("invalid file data: {e}"))?;

    let safe_name = sanitize_filename(&file_name);
    let save_path = rfd::FileDialog::new().set_file_name(&safe_name).save_file();
    let Some(path) = save_path else {
        return Ok(None);
    };

    fs::write(&path, bytes).map_err(|e| format!("failed to save file: {e}"))?;
    Ok(Some(path.display().to_string()))
}

#[tauri::command]
fn save_download_file_to_path(
    file_name: String,
    data_base64: String,
    destination_dir: String,
) -> Result<String, String> {
    let bytes = general_purpose::STANDARD
        .decode(data_base64)
        .map_err(|e| format!("invalid file data: {e}"))?;
    let safe_name = sanitize_filename(&file_name);

    let base_dir = if destination_dir.trim().is_empty() {
        default_downloads_dir()
    } else {
        PathBuf::from(destination_dir.trim())
    };
    fs::create_dir_all(&base_dir).map_err(|e| format!("failed to create destination dir: {e}"))?;

    let destination = base_dir.join(safe_name);
    fs::write(&destination, bytes).map_err(|e| format!("failed to save file: {e}"))?;
    Ok(destination.display().to_string())
}

#[tauri::command]
fn apply_input_event(app: tauri::AppHandle, event: Value) -> Result<(), String> {
    if !event.is_object() {
        return Err("invalid input event payload".to_string());
    }

    let event_type = event
        .get("event_type")
        .and_then(Value::as_str)
        .ok_or_else(|| "event_type is required".to_string())?;

    let mut enigo = Enigo::new(&Settings::default())
        .map_err(|e| format!("failed to init native input layer: {e}"))?;

    match event_type {
        "mouse_move" => {
            let (x, y) = normalized_to_screen(&app, &event)?;
            enigo
                .move_mouse(x, y, Coordinate::Abs)
                .map_err(|e| format!("mouse move failed: {e}"))?;
        }
        "mouse_down" | "mouse_up" => {
            let (x, y) = normalized_to_screen(&app, &event)?;
            enigo
                .move_mouse(x, y, Coordinate::Abs)
                .map_err(|e| format!("mouse move failed: {e}"))?;

            let button = parse_mouse_button(event.get("button"));
            let direction = if event_type == "mouse_down" {
                Direction::Press
            } else {
                Direction::Release
            };
            enigo
                .button(button, direction)
                .map_err(|e| format!("mouse button failed: {e}"))?;
        }
        "mouse_wheel" => {
            let delta_y = event.get("delta_y").and_then(Value::as_f64).unwrap_or(0.0);
            let steps = (delta_y / 120.0).round() as i32;
            if steps != 0 {
                enigo
                    .scroll(steps, Axis::Vertical)
                    .map_err(|e| format!("mouse wheel failed: {e}"))?;
            }
        }
        "key_down" | "key_up" => {
            let key = event.get("key").and_then(Value::as_str).unwrap_or("");
            let code = event.get("code").and_then(Value::as_str).unwrap_or("");
            let mapped_key = map_key(key, code)
                .ok_or_else(|| format!("unsupported key event (key={key}, code={code})"))?;

            let direction = if event_type == "key_down" {
                Direction::Press
            } else {
                Direction::Release
            };
            enigo
                .key(mapped_key, direction)
                .map_err(|e| format!("keyboard event failed: {e}"))?;
        }
        _ => {
            return Err(format!("unsupported event_type: {event_type}"));
        }
    }

    Ok(())
}

fn normalized_to_screen(app: &tauri::AppHandle, event: &Value) -> Result<(i32, i32), String> {
    let x = event
        .get("x")
        .and_then(Value::as_f64)
        .unwrap_or(0.0)
        .clamp(0.0, 1.0);
    let y = event
        .get("y")
        .and_then(Value::as_f64)
        .unwrap_or(0.0)
        .clamp(0.0, 1.0);

    let monitor = app
        .primary_monitor()
        .map_err(|e| format!("cannot read primary monitor: {e}"))?
        .ok_or_else(|| "primary monitor not available".to_string())?;

    let size = monitor.size();
    let width = (size.width.saturating_sub(1)) as f64;
    let height = (size.height.saturating_sub(1)) as f64;

    let screen_x = (x * width).round() as i32;
    let screen_y = (y * height).round() as i32;
    Ok((screen_x, screen_y))
}

fn parse_mouse_button(value: Option<&Value>) -> Button {
    match value.and_then(Value::as_i64).unwrap_or(0) {
        1 => Button::Middle,
        2 => Button::Right,
        _ => Button::Left,
    }
}

fn map_key(key: &str, code: &str) -> Option<Key> {
    if key.chars().count() == 1 {
        return key.chars().next().map(Key::Unicode);
    }

    let lowered = key.to_lowercase();
    let from_key = match lowered.as_str() {
        "enter" => Some(Key::Return),
        "tab" => Some(Key::Tab),
        "escape" => Some(Key::Escape),
        "backspace" => Some(Key::Backspace),
        " " | "space" | "spacebar" => Some(Key::Space),
        "arrowup" => Some(Key::UpArrow),
        "arrowdown" => Some(Key::DownArrow),
        "arrowleft" => Some(Key::LeftArrow),
        "arrowright" => Some(Key::RightArrow),
        "home" => Some(Key::Home),
        "end" => Some(Key::End),
        "pageup" => Some(Key::PageUp),
        "pagedown" => Some(Key::PageDown),
        "delete" => Some(Key::Delete),
        "insert" => Some(Key::Insert),
        "shift" => Some(Key::Shift),
        "control" | "ctrl" => Some(Key::Control),
        "alt" => Some(Key::Alt),
        "meta" | "os" | "super" => Some(Key::Meta),
        "f1" => Some(Key::F1),
        "f2" => Some(Key::F2),
        "f3" => Some(Key::F3),
        "f4" => Some(Key::F4),
        "f5" => Some(Key::F5),
        "f6" => Some(Key::F6),
        "f7" => Some(Key::F7),
        "f8" => Some(Key::F8),
        "f9" => Some(Key::F9),
        "f10" => Some(Key::F10),
        "f11" => Some(Key::F11),
        "f12" => Some(Key::F12),
        _ => None,
    };
    if from_key.is_some() {
        return from_key;
    }

    if let Some(c) = code.strip_prefix("Key").and_then(|s| s.chars().next()) {
        return Some(Key::Unicode(c.to_ascii_lowercase()));
    }
    if let Some(c) = code.strip_prefix("Digit").and_then(|s| s.chars().next()) {
        return Some(Key::Unicode(c));
    }

    match code {
        "Enter" | "NumpadEnter" => Some(Key::Return),
        "Tab" => Some(Key::Tab),
        "Escape" => Some(Key::Escape),
        "Backspace" => Some(Key::Backspace),
        "Space" => Some(Key::Space),
        "ArrowUp" => Some(Key::UpArrow),
        "ArrowDown" => Some(Key::DownArrow),
        "ArrowLeft" => Some(Key::LeftArrow),
        "ArrowRight" => Some(Key::RightArrow),
        "ShiftLeft" | "ShiftRight" => Some(Key::Shift),
        "ControlLeft" | "ControlRight" => Some(Key::Control),
        "AltLeft" | "AltRight" => Some(Key::Alt),
        "MetaLeft" | "MetaRight" => Some(Key::Meta),
        _ => None,
    }
}

fn sanitize_filename(name: &str) -> String {
    let candidate = Path::new(name)
        .file_name()
        .and_then(|part| part.to_str())
        .map(str::trim)
        .unwrap_or_default();
    if candidate.is_empty() {
        "file.bin".to_string()
    } else {
        candidate.to_string()
    }
}

fn default_downloads_dir() -> PathBuf {
    if let Ok(profile) = std::env::var("USERPROFILE") {
        let path = PathBuf::from(profile).join("Downloads");
        if fs::create_dir_all(&path).is_ok() {
            return path;
        }
    }

    if let Ok(home) = std::env::var("HOME") {
        let path = PathBuf::from(home).join("Downloads");
        if fs::create_dir_all(&path).is_ok() {
            return path;
        }
    }

    std::env::temp_dir()
}

pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            get_runtime_info,
            capture_primary_screen_jpeg,
            save_download_file,
            save_download_file_to_path,
            apply_input_event
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

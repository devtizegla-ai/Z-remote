use serde::Serialize;
use serde_json::Value;

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
fn apply_input_event(event: Value) -> Result<(), String> {
    // TODO: Integrar camada nativa por plataforma (Windows/macOS/Linux) para
    // mover mouse, clique e teclado de forma segura e auditável.
    // No MVP atual, o evento é apenas validado/registrado no cliente host.
    if !event.is_object() {
        return Err("invalid input event payload".to_string());
    }
    println!("[input-event] {:?}", event);
    Ok(())
}

pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![get_runtime_info, apply_input_event])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}


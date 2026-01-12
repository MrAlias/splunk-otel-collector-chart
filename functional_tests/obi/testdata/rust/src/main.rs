use axum::{
    extract::Json,
    http::{HeaderMap, HeaderName, StatusCode},
    response::IntoResponse,
    routing::{get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use std::env;

#[derive(Debug, Serialize, Deserialize)]
struct ChainRequest {
    targets: Vec<String>,
}

#[derive(Debug, Serialize, Deserialize)]
struct ChainResponse {
    service: String,
    status: u16,
    targets: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

#[tokio::main]
async fn main() {
    let port = env::var("SERVER_PORT").unwrap_or_else(|_| "8080".to_string());
    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{}", port))
        .await
        .unwrap();

    eprintln!("Starting server on port {}", port);

    let app = Router::new()
        .route("/health", get(handle_health))
        .route("/chain", post(handle_chain));

    axum::serve(listener, app).await.unwrap();
}

async fn handle_health() -> impl IntoResponse {
    (StatusCode::OK, Json(serde_json::json!({"status": "ok"})))
}

async fn handle_chain(
    headers: HeaderMap,
    Json(payload): Json<ChainRequest>,
) -> impl IntoResponse {
    let targets = payload.targets;

    eprintln!("Received chain request with {} targets", targets.len());

    // If no targets, return success (end of chain)
    if targets.is_empty() {
        let response = ChainResponse {
            service: "rust".to_string(),
            status: 200,
            targets: targets.clone(),
            result: Some("Chain completed".to_string()),
            error: None,
        };
        return (StatusCode::OK, Json(response)).into_response();
    }

    // Forward to next target with remaining targets
    let next_target = &targets[0];
    let remaining_targets = targets[1..].to_vec();

    eprintln!(
        "Forwarding to {} with {} remaining targets",
        next_target,
        remaining_targets.len()
    );

    let next_req = ChainRequest {
        targets: remaining_targets,
    };

    match make_request(next_target, next_req).await {
        Ok(response_body) => {
            match serde_json::from_str::<serde_json::Value>(&response_body) {
                Ok(json_response) => (StatusCode::OK, Json(json_response)).into_response(),
                Err(_) => {
                    let response = ChainResponse {
                        service: "rust".to_string(),
                        status: 502,
                        targets: targets.clone(),
                        result: None,
                        error: Some(format!("Failed to parse response from {}", next_target)),
                    };
                    (StatusCode::BAD_GATEWAY, Json(response)).into_response()
                }
            }
        }
        Err(e) => {
            let response = ChainResponse {
                service: "rust".to_string(),
                status: 502,
                targets: targets.clone(),
                result: None,
                error: Some(format!("Failed to call {}: {}", next_target, e)),
            };
            (StatusCode::BAD_GATEWAY, Json(response)).into_response()
        }
    }
}

async fn make_request(target: &str, data: ChainRequest) -> Result<String, String> {
    let client = reqwest::Client::new();
    let chain_url = format!("http://{}/chain", target);

    let response = client.post(&chain_url)
        .json(&data)
        .timeout(std::time::Duration::from_secs(10))
        .send()
        .await
        .map_err(|e| format!("{}", e))?;

    response.text().await.map_err(|e| format!("{}", e))
}

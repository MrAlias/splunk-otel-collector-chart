import com.sun.net.httpserver.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.*;
import org.json.JSONArray;
import org.json.JSONObject;

public class ChainServer {
    private static HttpServer server;
    private static final String SERVICE_NAME = "java";

    public static void main(String[] args) throws Exception {
        String port = System.getenv("SERVER_PORT");
        if (port == null || port.isEmpty()) {
            port = "8080";
        }

        int serverPort = Integer.parseInt(port);
        server = HttpServer.create(new InetSocketAddress(serverPort), 0);

        // Register handlers
        server.createContext("/health", ChainServer::handleHealth);
        server.createContext("/chain", ChainServer::handleChain);
        server.createContext("/", ChainServer::handle404);

        server.setExecutor(java.util.concurrent.Executors.newFixedThreadPool(10));
        server.start();

        System.out.println("Starting server on port " + serverPort);
    }

    private static void handleHealth(HttpExchange exchange) throws IOException {
        String response = "{\"status\": \"ok\"}";
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(200, response.getBytes(StandardCharsets.UTF_8).length);
        exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
        exchange.close();
    }

    private static void handleChain(HttpExchange exchange) throws IOException {
        try {
            if (!exchange.getRequestMethod().equals("POST")) {
                String response = buildErrorResponse(405, "Only POST requests are allowed");
                exchange.getResponseHeaders().set("Content-Type", "application/json");
                exchange.sendResponseHeaders(405, response.getBytes(StandardCharsets.UTF_8).length);
                exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
                exchange.close();
                return;
            }

            // Read request body
            String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
            JSONObject req = new JSONObject(body);
            JSONArray targetArray = req.getJSONArray("targets");
            List<String> targets = new ArrayList<>();
            for (int i = 0; i < targetArray.length(); i++) {
                targets.add(targetArray.getString(i));
            }

            System.err.println("Received chain request with " + targets.size() + " targets");

            // If no targets, return success (end of chain)
            if (targets.isEmpty()) {
                String response = buildSuccessResponse(200, targets, "Chain completed");
                exchange.getResponseHeaders().set("Content-Type", "application/json");
                exchange.sendResponseHeaders(200, response.getBytes(StandardCharsets.UTF_8).length);
                exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
                exchange.close();
                return;
            }

            // Forward to next target with remaining targets
            String nextTarget = targets.get(0);
            List<String> remainingTargets = targets.subList(1, targets.size());

            System.err.println("Forwarding to " + nextTarget + " with " + remainingTargets.size() + " remaining targets");

            JSONObject nextReq = new JSONObject();
            nextReq.put("targets", remainingTargets);

            try {
                String chainUrl = "http://" + nextTarget + "/chain";
                HttpURLConnection conn = (HttpURLConnection) new URL(chainUrl).openConnection();
                conn.setRequestMethod("POST");
                conn.setRequestProperty("Content-Type", "application/json");
                conn.setConnectTimeout(10000);
                conn.setReadTimeout(10000);
                conn.setDoOutput(true);

                byte[] requestBody = nextReq.toString().getBytes(StandardCharsets.UTF_8);
                conn.setFixedLengthStreamingMode(requestBody.length);
                try (OutputStream os = conn.getOutputStream()) {
                    os.write(requestBody);
                    os.flush();
                }

                int responseCode = conn.getResponseCode();
                InputStream is = responseCode >= 400 ? conn.getErrorStream() : conn.getInputStream();
                String responseBody = is != null
                    ? new String(is.readAllBytes(), StandardCharsets.UTF_8)
                    : "";

                exchange.getResponseHeaders().set("Content-Type", "application/json");
                byte[] responseBytes = responseBody.getBytes(StandardCharsets.UTF_8);
                exchange.sendResponseHeaders(responseCode, responseBytes.length);
                exchange.getResponseBody().write(responseBytes);
                exchange.close();

            } catch (Exception e) {
                String response = buildErrorResponse(502, "Failed to call " + nextTarget + ": " + e.getMessage());
                exchange.getResponseHeaders().set("Content-Type", "application/json");
                exchange.sendResponseHeaders(502, response.getBytes(StandardCharsets.UTF_8).length);
                exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
                exchange.close();
            }

        } catch (Exception e) {
            String response = buildErrorResponse(400, "Error processing request: " + e.getMessage());
            exchange.getResponseHeaders().set("Content-Type", "application/json");
            exchange.sendResponseHeaders(400, response.getBytes(StandardCharsets.UTF_8).length);
            exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
            exchange.close();
        }
    }

    private static void handle404(HttpExchange exchange) throws IOException {
        String response = "{\"error\": \"Not found\"}";
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(404, response.getBytes(StandardCharsets.UTF_8).length);
        exchange.getResponseBody().write(response.getBytes(StandardCharsets.UTF_8));
        exchange.close();
    }

    private static String buildSuccessResponse(int status, List<String> targets, String result) {
        JSONObject response = new JSONObject();
        response.put("service", SERVICE_NAME);
        response.put("status", status);
        response.put("targets", targets);
        response.put("result", result);
        return response.toString();
    }

    private static String buildErrorResponse(int status, String error) {
        JSONObject response = new JSONObject();
        response.put("service", SERVICE_NAME);
        response.put("status", status);
        response.put("error", error);
        return response.toString();
    }
}

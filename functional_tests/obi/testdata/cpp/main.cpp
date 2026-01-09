#include <iostream>
#include <string>
#include <vector>
#include <memory>
#include <cstring>
#include <sstream>
#include <unistd.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <nlohmann/json.hpp>

using json = nlohmann::json;

const std::string SERVICE_NAME = "cpp";
const int BUFFER_SIZE = 65536;

class HttpServer {
private:
    int server_socket;
    int port;
    
public:
    HttpServer(int p) : port(p) {}
    
    void start() {
        server_socket = socket(AF_INET, SOCK_STREAM, 0);
        if (server_socket < 0) {
            std::cerr << "Error creating socket\n";
            exit(1);
        }
        
        int reuse = 1;
        setsockopt(server_socket, SOL_SOCKET, SO_REUSEADDR, (const char*)&reuse, sizeof(reuse));
        
        sockaddr_in addr;
        addr.sin_family = AF_INET;
        addr.sin_addr.s_addr = INADDR_ANY;
        addr.sin_port = htons(port);
        
        if (bind(server_socket, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            std::cerr << "Error binding socket\n";
            exit(1);
        }
        
        listen(server_socket, 5);
        std::cerr << "Starting server on port " << port << "\n";
        
        while (true) {
            sockaddr_in client_addr;
            socklen_t client_len = sizeof(client_addr);
            int client_socket = accept(server_socket, (struct sockaddr*)&client_addr, &client_len);
            
            if (client_socket < 0) continue;
            
            handle_request(client_socket);
            close(client_socket);
        }
    }
    
private:
    void handle_request(int client_socket) {
        // Read the entire request including headers and body
        std::string request;
        char buffer[BUFFER_SIZE] = {0};
        int bytes_read = read(client_socket, buffer, BUFFER_SIZE - 1);
        
        if (bytes_read < 0) {
            send_response(client_socket, 400, "Error reading request");
            return;
        }
        
        request.append(buffer, bytes_read);
        
        // Parse headers to get Content-Length
        size_t header_end = request.find("\r\n\r\n");
        if (header_end == std::string::npos) {
            send_response(client_socket, 400, "Invalid request format");
            return;
        }
        
        std::string headers_str = request.substr(0, header_end);
        int content_length = 0;
        size_t cl_pos = headers_str.find("Content-Length:");
        if (cl_pos != std::string::npos) {
            size_t cl_end = headers_str.find("\r\n", cl_pos);
            std::string cl_value = headers_str.substr(cl_pos + 15, cl_end - (cl_pos + 15));
            // Trim whitespace
            cl_value.erase(0, cl_value.find_first_not_of(" \t"));
            cl_value.erase(cl_value.find_last_not_of(" \t") + 1);
            content_length = std::atoi(cl_value.c_str());
        }
        
        // Read remaining body if needed
        size_t body_start = header_end + 4;
        int current_body_size = request.size() - body_start;
        
        while (current_body_size < content_length) {
            memset(buffer, 0, sizeof(buffer));
            bytes_read = read(client_socket, buffer, BUFFER_SIZE - 1);
            if (bytes_read <= 0) {
                break;
            }
            request.append(buffer, bytes_read);
            current_body_size += bytes_read;
        }
        
        if (request.find("GET /health") == 0) {
            handle_health(client_socket);
        } else if (request.find("POST /chain") == 0) {
            handle_chain(client_socket, request);
        } else {
            send_response(client_socket, 404, "Not found");
        }
    }
    
    void handle_health(int client_socket) {
        json response;
        response["status"] = "ok";
        send_response(client_socket, 200, response.dump());
    }
    
    void handle_chain(int client_socket, const std::string& request) {
        try {
            // Extract body from request
            size_t body_start = request.find("\r\n\r\n");
            if (body_start == std::string::npos) {
                send_response(client_socket, 400, "Invalid request format");
                return;
            }
            
            // Parse headers (simple)
            std::map<std::string, std::string> headers;
            {
                std::istringstream iss(request.substr(0, body_start));
                std::string line;
                // Skip request line
                std::getline(iss, line);
                while (std::getline(iss, line)) {
                    if (line.size() < 2) continue;
                    if (line == "\r") break;
                    size_t colon = line.find(':');
                    if (colon != std::string::npos) {
                        std::string key = line.substr(0, colon);
                        std::string val = line.substr(colon + 1);
                        // trim
                        key.erase(std::remove_if(key.begin(), key.end(), ::isspace), key.end());
                        while (!val.empty() && (val[0] == ' ')) val.erase(0, 1);
                        if (!val.empty() && val.back() == '\r') val.pop_back();
                        headers[key] = val;
                    }
                }
            }

            std::string body = request.substr(body_start + 4);
            json data = json::parse(body);
            std::vector<std::string> targets = data["targets"];
            
            std::cerr << "Received chain request with " << targets.size() << " targets\n";
            
            // If no targets, return success (end of chain)
            if (targets.empty()) {
                json response;
                response["service"] = SERVICE_NAME;
                response["status"] = 200;
                response["targets"] = targets;
                response["result"] = "Chain completed";
                send_response(client_socket, 200, response.dump());
                return;
            }
            
            // Forward to next target with remaining targets
            std::string next_target = targets[0];
            std::vector<std::string> remaining_targets(targets.begin() + 1, targets.end());
            
            std::cerr << "Forwarding to " << next_target << " with " << remaining_targets.size() << " remaining targets\n";
            
            json next_req;
            next_req["targets"] = remaining_targets;
            
            // Parse target host and port
            size_t colon_pos = next_target.rfind(':');
            std::string host, port_str;
            int port_num = 80;
            
            if (colon_pos != std::string::npos) {
                host = next_target.substr(0, colon_pos);
                port_str = next_target.substr(colon_pos + 1);
                port_num = std::stoi(port_str);
            } else {
                host = next_target;
            }
            
            if (forward_request(client_socket, host, port_num, next_req, headers)) {
                return;
            }
            
            json error_response;
            error_response["service"] = SERVICE_NAME;
            error_response["status"] = 502;
            error_response["targets"] = targets;
            error_response["error"] = "Failed to call " + next_target;
            send_response(client_socket, 502, error_response.dump());
            
        } catch (const std::exception& e) {
            json error_response;
            error_response["service"] = SERVICE_NAME;
            error_response["status"] = 400;
            error_response["error"] = std::string("Error processing request: ") + e.what();
            send_response(client_socket, 400, error_response.dump());
        }
    }
    
    bool forward_request(int client_socket, const std::string& host, int port_num, const json& data, const std::map<std::string, std::string>& in_headers) {
        int sock = socket(AF_INET, SOCK_STREAM, 0);
        if (sock < 0) return false;
        
        struct hostent* he = gethostbyname(host.c_str());
        if (!he) {
            close(sock);
            return false;
        }
        
        struct sockaddr_in addr;
        addr.sin_family = AF_INET;
        addr.sin_port = htons(port_num);
        addr.sin_addr = *(struct in_addr*)he->h_addr;
        
        if (connect(sock, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            close(sock);
            return false;
        }
        
        std::string body = data.dump();
        std::ostringstream reqbuf;
        reqbuf << "POST /chain HTTP/1.1\r\n";
        reqbuf << "Host: " << host << ":" << port_num << "\r\n";
        reqbuf << "Content-Type: application/json\r\n";
        reqbuf << "Content-Length: " << body.length() << "\r\n";
        // Forward trace headers
        const char* traceHeaders[] = {
            "traceparent", "tracestate",
            "b3", "x-b3-traceid", "x-b3-spanid", "x-b3-sampled",
            "x-ot-span-context"
        };
        for (const char* h : traceHeaders) {
            auto it = in_headers.find(h);
            if (it != in_headers.end()) {
                reqbuf << h << ": " << it->second << "\r\n";
            }
        }
        reqbuf << "Connection: close\r\n";
        reqbuf << "\r\n";
        reqbuf << body;
        
        std::string request_str = reqbuf.str();
        if (send(sock, request_str.c_str(), request_str.length(), 0) < 0) {
            close(sock);
            return false;
        }
        
        // Read response fully
        std::string response;
        char buffer[BUFFER_SIZE] = {0};
        int bytes_read = 0;
        int total_bytes = 0;
        while ((bytes_read = read(sock, buffer, BUFFER_SIZE - 1)) > 0) {
            response.append(buffer, bytes_read);
            total_bytes += bytes_read;
            memset(buffer, 0, sizeof(buffer));
        }
        close(sock);

        std::cerr << "Received response of " << total_bytes << " bytes\n";

        if (response.empty()) {
            std::cerr << "Empty response from upstream\n";
            return false;
        }

        // Extract status code
        int status_code = 200;
        size_t status_pos = response.find("HTTP/1.1 ");
        if (status_pos != std::string::npos) {
            size_t code_start = status_pos + 9;
            status_code = std::atoi(response.substr(code_start, 3).c_str());
        }

        size_t body_start = response.find("\r\n\r\n");
        if (body_start != std::string::npos) {
            std::string response_body = response.substr(body_start + 4);
            std::cerr << "Extracted body: " << response_body.length() << " bytes\n";
            // Validate the response body is not empty
            if (!response_body.empty() && response_body[0] != '\0') {
                send_response(client_socket, status_code, response_body);
                return true;
            }
        }

        std::cerr << "Failed to extract valid body from response\n";
        // If we couldn't extract a valid body, return an error
        json error_response;
        error_response["service"] = SERVICE_NAME;
        error_response["status"] = 502;
        error_response["error"] = "Empty or invalid response from upstream service";
        send_response(client_socket, 502, error_response.dump());
        return false;
    }
    
    void send_response(int client_socket, int status_code, const std::string& body) {
        std::ostringstream response;
        response << "HTTP/1.1 " << status_code << " OK\r\n";
        response << "Content-Type: application/json\r\n";
        response << "Content-Length: " << body.length() << "\r\n";
        response << "Connection: close\r\n";
        response << "\r\n";
        response << body;
        
        std::string response_str = response.str();
        send(client_socket, response_str.c_str(), response_str.length(), 0);
    }
};

int main() {
    const char* port_env = std::getenv("SERVER_PORT");
    int port = port_env ? std::stoi(port_env) : 8080;
    
    HttpServer server(port);
    server.start();
    
    return 0;
}

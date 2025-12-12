#!/usr/bin/env python3
import os
import sys
import json
import requests
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "ok"}), 200

@app.route('/chain', methods=['POST'])
def chain():
    try:
        data = request.get_json()
        targets = data.get('targets', [])
        
        print(f"Received chain request with {len(targets)} targets", file=sys.stderr)
        
        # If no targets, return success (end of chain)
        if not targets:
            return jsonify({
                "service": "python",
                "status": 200,
                "targets": targets,
                "result": "Chain completed"
            }), 200
        
        # Forward to next target with remaining targets
        next_target = targets[0]
        remaining_targets = targets[1:]
        
        print(f"Forwarding to {next_target} with {len(remaining_targets)} remaining targets", file=sys.stderr)
        
        next_req = {
            "targets": remaining_targets
        }
        
        try:
            chain_url = f"http://{next_target}/chain"
            # Forward trace headers
            trace_headers = [
                'traceparent', 'tracestate',
                'b3', 'x-b3-traceid', 'x-b3-spanid', 'x-b3-sampled',
                'x-ot-span-context'
            ]
            fwd_headers = {}
            for h in trace_headers:
                v = request.headers.get(h)
                if v:
                    fwd_headers[h] = v
            resp = requests.post(chain_url, json=next_req, headers=fwd_headers, timeout=10)
            
            # Return the response from next target
            return resp.json(), resp.status_code
            
        except requests.exceptions.RequestException as e:
            return jsonify({
                "service": "python",
                "status": 502,
                "targets": targets,
                "error": f"Failed to call {next_target}: {str(e)}"
            }), 502
    
    except Exception as e:
        return jsonify({
            "service": "python",
            "status": 400,
            "error": f"Error processing request: {str(e)}"
        }), 400

@app.errorhandler(405)
def method_not_allowed(e):
    return jsonify({
        "service": "python",
        "status": 405,
        "error": "Only POST requests are allowed"
    }), 405

if __name__ == '__main__':
    port = os.getenv('SERVER_PORT', '8080')
    print(f"Starting server on port {port}", file=sys.stderr)
    app.run(host='0.0.0.0', port=int(port), debug=False)

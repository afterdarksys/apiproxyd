#!/usr/bin/env python3
"""
Example Python plugin for apiproxyd that logs all requests and responses.
This plugin communicates with the Go daemon via JSON-RPC over stdin/stdout.
"""

import json
import sys
from datetime import datetime


class LoggerPlugin:
    def __init__(self):
        self.config = {}

    def get_info(self):
        """Return plugin information"""
        return {
            "name": "python_logger",
            "version": "1.0.0"
        }

    def init(self, config):
        """Initialize the plugin with configuration"""
        self.config = config
        self.log(f"Initialized with config: {config}")
        return {"status": "ok"}

    def on_request(self, request_json):
        """Called before proxying the request"""
        request = json.loads(request_json)
        self.log(f"{request['method']} Request to {request['endpoint']} at {datetime.now().isoformat()}")

        # Add custom header
        if 'headers' not in request:
            request['headers'] = {}
        request['headers']['X-Plugin-Python-Logger'] = 'enabled'

        return {
            "request": request,
            "continue": True
        }

    def on_response(self, request_json, response_json):
        """Called after receiving the upstream response"""
        request = json.loads(request_json)
        response = json.loads(response_json)

        self.log(f"Response from {request['endpoint']}: status={response['status_code']}, size={len(response['body'])} bytes")

        # Add metadata
        if 'metadata' not in response:
            response['metadata'] = {}
        response['metadata']['logged_at'] = datetime.now().isoformat()

        return response

    def on_cache_hit(self, request_json, response_json):
        """Called when a cached response is found"""
        request = json.loads(request_json)
        response = json.loads(response_json)

        self.log(f"Cache HIT for {request['method']} {request['endpoint']}")
        return response

    def shutdown(self):
        """Gracefully shut down the plugin"""
        self.log("Shutting down")
        return {"status": "ok"}

    def log(self, message):
        """Log a message to stderr"""
        print(f"[Python Logger Plugin] {message}", file=sys.stderr, flush=True)


def handle_rpc_call(plugin, method, params):
    """Handle a JSON-RPC call"""
    if method == "get_info":
        return plugin.get_info()
    elif method == "init":
        return plugin.init(params[0] if params else {})
    elif method == "on_request":
        return plugin.on_request(params[0])
    elif method == "on_response":
        return plugin.on_response(params[0], params[1])
    elif method == "on_cache_hit":
        return plugin.on_cache_hit(params[0], params[1])
    elif method == "shutdown":
        return plugin.shutdown()
    else:
        raise ValueError(f"Unknown method: {method}")


def main():
    plugin = LoggerPlugin()

    # Read JSON-RPC requests from stdin and write responses to stdout
    for line in sys.stdin:
        try:
            request = json.loads(line.strip())

            # Validate JSON-RPC request
            if request.get("jsonrpc") != "2.0":
                continue

            method = request.get("method")
            params = request.get("params", [])
            req_id = request.get("id")

            # Handle the RPC call
            result = handle_rpc_call(plugin, method, params)

            # Send response
            response = {
                "jsonrpc": "2.0",
                "result": result,
                "id": req_id
            }
            print(json.dumps(response), flush=True)

        except Exception as e:
            # Send error response
            response = {
                "jsonrpc": "2.0",
                "error": {
                    "code": -32000,
                    "message": str(e)
                },
                "id": request.get("id") if 'request' in locals() else None
            }
            print(json.dumps(response), flush=True)


if __name__ == "__main__":
    main()

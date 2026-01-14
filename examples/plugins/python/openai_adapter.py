#!/usr/bin/env python3
"""
Example Python plugin that adapts OpenAI API requests to apiproxyd.
This allows you to use apiproxyd as a proxy for OpenAI API with custom caching,
rate limiting, and monitoring.
"""

import json
import sys
import base64
from datetime import datetime


class OpenAIAdapterPlugin:
    def __init__(self):
        self.config = {}
        self.api_key = None

    def get_info(self):
        """Return plugin information"""
        return {
            "name": "openai_adapter",
            "version": "1.0.0"
        }

    def init(self, config):
        """Initialize the plugin with configuration"""
        self.config = config
        self.api_key = config.get("openai_api_key", "")
        self.log(f"Initialized OpenAI adapter")
        return {"status": "ok"}

    def on_request(self, request_json):
        """Transform OpenAI API requests"""
        request = json.loads(request_json)

        # Check if this is an OpenAI endpoint
        if not request['endpoint'].startswith('/v1/openai/'):
            return {"request": request, "continue": True}

        self.log(f"Processing OpenAI request to {request['endpoint']}")

        # Add OpenAI authentication header if configured
        if self.api_key:
            if 'headers' not in request:
                request['headers'] = {}
            request['headers']['Authorization'] = f'Bearer {self.api_key}'

        # Transform the endpoint to actual OpenAI API
        # /v1/openai/chat/completions -> /v1/chat/completions
        original_endpoint = request['endpoint']
        request['endpoint'] = original_endpoint.replace('/v1/openai/', '/v1/')

        # Store original endpoint for later
        if 'metadata' not in request:
            request['metadata'] = {}
        request['metadata']['original_endpoint'] = original_endpoint
        request['metadata']['provider'] = 'openai'

        # Parse and potentially modify the request body
        if request.get('body'):
            try:
                body = json.loads(request['body']) if isinstance(request['body'], str) else request['body']

                # Add custom parameters or transform the request
                if 'model' not in body:
                    body['model'] = 'gpt-3.5-turbo'

                # Add usage tracking metadata
                body['user'] = body.get('user', f"apiproxyd-{datetime.now().strftime('%Y%m%d')}")

                request['body'] = json.dumps(body).encode('utf-8') if isinstance(request['body'], bytes) else json.dumps(body)

                self.log(f"Transformed request for model: {body.get('model')}")
            except json.JSONDecodeError:
                self.log("Warning: Could not parse request body as JSON")

        return {"request": request, "continue": True}

    def on_response(self, request_json, response_json):
        """Transform OpenAI API responses"""
        request = json.loads(request_json)
        response = json.loads(response_json)

        # Only process OpenAI responses
        if request.get('metadata', {}).get('provider') != 'openai':
            return response

        try:
            # Parse the response body
            body = json.loads(response['body']) if isinstance(response['body'], str) else response['body']

            # Add custom metadata
            if 'metadata' not in response:
                response['metadata'] = {}

            # Extract usage information
            if 'usage' in body:
                response['metadata']['tokens_used'] = str(body['usage'].get('total_tokens', 0))
                response['metadata']['prompt_tokens'] = str(body['usage'].get('prompt_tokens', 0))
                response['metadata']['completion_tokens'] = str(body['usage'].get('completion_tokens', 0))

            # Extract model information
            if 'model' in body:
                response['metadata']['model'] = body['model']

            self.log(f"Response processed: tokens={response['metadata'].get('tokens_used', 'unknown')}")

        except json.JSONDecodeError:
            self.log("Warning: Could not parse response body as JSON")

        return response

    def on_cache_hit(self, request_json, response_json):
        """Handle cached OpenAI responses"""
        request = json.loads(request_json)
        response = json.loads(response_json)

        if request.get('metadata', {}).get('provider') == 'openai':
            self.log(f"Cache HIT for OpenAI request - saved API costs!")

            # Add cache metadata
            if 'metadata' not in response:
                response['metadata'] = {}
            response['metadata']['cached'] = 'true'
            response['metadata']['cache_hit_at'] = datetime.now().isoformat()

        return response

    def shutdown(self):
        """Gracefully shut down the plugin"""
        self.log("Shutting down OpenAI adapter")
        return {"status": "ok"}

    def log(self, message):
        """Log a message to stderr"""
        print(f"[OpenAI Adapter] {message}", file=sys.stderr, flush=True)


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
    plugin = OpenAIAdapterPlugin()

    # Read JSON-RPC requests from stdin and write responses to stdout
    for line in sys.stdin:
        try:
            request = json.loads(line.strip())

            if request.get("jsonrpc") != "2.0":
                continue

            method = request.get("method")
            params = request.get("params", [])
            req_id = request.get("id")

            result = handle_rpc_call(plugin, method, params)

            response = {
                "jsonrpc": "2.0",
                "result": result,
                "id": req_id
            }
            print(json.dumps(response), flush=True)

        except Exception as e:
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

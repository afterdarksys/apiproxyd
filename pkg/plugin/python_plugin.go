package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// PythonPlugin wraps a Python plugin executed as a subprocess
type PythonPlugin struct {
	name    string
	version string
	path    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	scanner *bufio.Scanner
	mu      sync.Mutex
	reqID   int
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LoadPythonPlugin loads a Python plugin from the specified path
func LoadPythonPlugin(path string, config map[string]interface{}) (Plugin, error) {
	cmd := exec.Command("python3", path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start plugin process: %w", err)
	}

	// Start stderr reader in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("[Plugin %s stderr] %s\n", path, scanner.Text())
		}
	}()

	p := &PythonPlugin{
		path:    path,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		scanner: bufio.NewScanner(stdout),
		reqID:   0,
	}

	// Get plugin info
	info, err := p.call("get_info", nil)
	if err != nil {
		p.Shutdown()
		return nil, fmt.Errorf("failed to get plugin info: %w", err)
	}

	var infoMap map[string]string
	if err := json.Unmarshal(info, &infoMap); err != nil {
		p.Shutdown()
		return nil, fmt.Errorf("failed to parse plugin info: %w", err)
	}

	p.name = infoMap["name"]
	p.version = infoMap["version"]

	return p, nil
}

func (p *PythonPlugin) Name() string {
	return p.name
}

func (p *PythonPlugin) Version() string {
	return p.version
}

func (p *PythonPlugin) Init(config map[string]interface{}) error {
	_, err := p.call("init", []interface{}{config})
	return err
}

func (p *PythonPlugin) OnRequest(ctx context.Context, req *Request) (*Request, bool, error) {
	reqJSON, err := req.ToJSON()
	if err != nil {
		return req, false, err
	}

	result, err := p.call("on_request", []interface{}{string(reqJSON)})
	if err != nil {
		return req, false, err
	}

	var response struct {
		Request  json.RawMessage `json:"request"`
		Continue bool            `json:"continue"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return req, false, err
	}

	var modifiedReq Request
	if err := modifiedReq.FromJSON(response.Request); err != nil {
		return req, false, err
	}

	return &modifiedReq, response.Continue, nil
}

func (p *PythonPlugin) OnResponse(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	reqJSON, err := req.ToJSON()
	if err != nil {
		return resp, err
	}

	respJSON, err := resp.ToJSON()
	if err != nil {
		return resp, err
	}

	result, err := p.call("on_response", []interface{}{string(reqJSON), string(respJSON)})
	if err != nil {
		return resp, err
	}

	var modifiedResp Response
	if err := modifiedResp.FromJSON(result); err != nil {
		return resp, err
	}

	return &modifiedResp, nil
}

func (p *PythonPlugin) OnCacheHit(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	reqJSON, err := req.ToJSON()
	if err != nil {
		return resp, err
	}

	respJSON, err := resp.ToJSON()
	if err != nil {
		return resp, err
	}

	result, err := p.call("on_cache_hit", []interface{}{string(reqJSON), string(respJSON)})
	if err != nil {
		return resp, err
	}

	var modifiedResp Response
	if err := modifiedResp.FromJSON(result); err != nil {
		return resp, err
	}

	return &modifiedResp, nil
}

func (p *PythonPlugin) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stdin != nil {
		// Send shutdown command
		p.call("shutdown", nil)
		p.stdin.Close()
	}

	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}

	return nil
}

// call makes a JSON-RPC call to the Python plugin
func (p *PythonPlugin) call(method string, params []interface{}) (json.RawMessage, error) {
	p.mu.Lock()
	p.reqID++
	id := p.reqID
	p.mu.Unlock()

	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Send request
	p.mu.Lock()
	_, err = p.stdin.Write(append(reqJSON, '\n'))
	p.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	p.mu.Lock()
	if !p.scanner.Scan() {
		p.mu.Unlock()
		if err := p.scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("plugin closed connection")
	}
	line := p.scanner.Text()
	p.mu.Unlock()

	var resp RPCResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("plugin error: %s (code %d)", resp.Error.Message, resp.Error.Code)
	}

	return resp.Result, nil
}

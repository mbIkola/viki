//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

type mcpSmokeResult struct {
	Tools     []string
	Prompts   []string
	Templates []string
}

func TestMCPBinarySmoke(t *testing.T) {
	got, err := probeMCPBinarySmoke(defaultMCPBinaryPath(), defaultMCPConfigPath())
	if err != nil {
		t.Fatalf("probe mcp binary smoke: %v", err)
	}

	assertSameMembers(t, "tools", got.Tools, []string{"search", "ask", "get_tree", "what_changed"})
	assertSameMembers(t, "prompts", got.Prompts, []string{"daily_brief", "investigate_page", "compare_versions"})
	assertSameMembers(t, "resource templates", got.Templates, []string{
		"confluence://page/{page_id}",
		"confluence://chunk/{chunk_id}",
		"confluence://digest/{date}",
	})
}

func defaultMCPBinaryPath() string {
	if path := os.Getenv("MCP_BINARY"); path != "" {
		return path
	}
	return "../bin/mcp"
}

func defaultMCPConfigPath() string {
	if path := os.Getenv("MCP_CONFIG"); path != "" {
		return path
	}
	return "testdata/mcp_smoke_config.yaml"
}

func probeMCPBinarySmoke(binaryPath, configPath string) (mcpSmokeResult, error) {
	if _, err := os.Stat(binaryPath); err != nil {
		return mcpSmokeResult{}, fmt.Errorf("stat binary %q: %w", binaryPath, err)
	}
	if _, err := os.Stat(configPath); err != nil {
		return mcpSmokeResult{}, fmt.Errorf("stat config %q: %w", configPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--quiet", "--config", configPath)
	var stderr stderrBuffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return mcpSmokeResult{}, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return mcpSmokeResult{}, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return mcpSmokeResult{}, fmt.Errorf("start mcp binary: %w", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	lines := streamJSONLines(stdout)
	if _, err := rpcCall(stdin, lines, 1, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "mcp-binary-smoke",
			"version": "1",
		},
	}, stderr.String); err != nil {
		return mcpSmokeResult{}, err
	}
	if err := writeMessage(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}); err != nil {
		return mcpSmokeResult{}, fmt.Errorf("write initialized notification: %w", err)
	}

	toolsResult, err := rpcCall(stdin, lines, 2, "tools/list", map[string]any{}, stderr.String)
	if err != nil {
		return mcpSmokeResult{}, err
	}
	resourcesResult, err := rpcCall(stdin, lines, 3, "resources/list", map[string]any{}, stderr.String)
	if err != nil {
		return mcpSmokeResult{}, err
	}
	templatesResult, err := rpcCall(stdin, lines, 4, "resources/templates/list", map[string]any{}, stderr.String)
	if err != nil {
		return mcpSmokeResult{}, err
	}
	promptsResult, err := rpcCall(stdin, lines, 5, "prompts/list", map[string]any{}, stderr.String)
	if err != nil {
		return mcpSmokeResult{}, err
	}

	toolNames, err := decodeNamedList(toolsResult["tools"], "tools")
	if err != nil {
		return mcpSmokeResult{}, err
	}
	templateURIs, err := decodeTemplateList(templatesResult["resourceTemplates"])
	if err != nil {
		return mcpSmokeResult{}, err
	}
	promptNames, err := decodeNamedList(promptsResult["prompts"], "prompts")
	if err != nil {
		return mcpSmokeResult{}, err
	}
	if _, err := decodeResourceCount(resourcesResult["resources"]); err != nil {
		return mcpSmokeResult{}, err
	}

	return mcpSmokeResult{
		Tools:     toolNames,
		Prompts:   promptNames,
		Templates: templateURIs,
	}, nil
}

func assertSameMembers(t *testing.T, label string, got []string, want []string) {
	t.Helper()
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	slices.Sort(gotCopy)
	slices.Sort(wantCopy)
	if !slices.Equal(gotCopy, wantCopy) {
		t.Fatalf("unexpected %s: got=%v want=%v", label, gotCopy, wantCopy)
	}
}

type scanEvent struct {
	line []byte
	err  error
	eof  bool
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func streamJSONLines(stdout interface{ Read([]byte) (int, error) }) <-chan scanEvent {
	ch := make(chan scanEvent)
	go func() {
		defer close(ch)

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			ch <- scanEvent{line: line}
		}
		if err := scanner.Err(); err != nil {
			ch <- scanEvent{err: err}
			return
		}
		ch <- scanEvent{eof: true}
	}()
	return ch
}

func rpcCall(stdin interface{ Write([]byte) (int, error) }, lines <-chan scanEvent, id int, method string, params map[string]any, stderr func() string) (map[string]json.RawMessage, error) {
	if err := writeMessage(stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return nil, fmt.Errorf("write %s request: %w", method, err)
	}

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case event, ok := <-lines:
			if !ok {
				return nil, fmt.Errorf("%s response channel closed; stderr=%s", method, stderr())
			}
			if event.err != nil {
				return nil, fmt.Errorf("read %s response: %w; stderr=%s", method, event.err, stderr())
			}
			if event.eof {
				return nil, fmt.Errorf("mcp process ended before %s response; stderr=%s", method, stderr())
			}

			var msg map[string]json.RawMessage
			if err := json.Unmarshal(event.line, &msg); err != nil {
				return nil, fmt.Errorf("decode rpc line for %s: %w; line=%s", method, err, string(event.line))
			}
			idRaw, hasID := msg["id"]
			if !hasID {
				continue
			}
			var gotID int
			if err := json.Unmarshal(idRaw, &gotID); err != nil {
				return nil, fmt.Errorf("decode rpc id for %s: %w", method, err)
			}
			if gotID != id {
				continue
			}
			if errRaw, ok := msg["error"]; ok && string(errRaw) != "null" {
				var rpcErr rpcError
				if err := json.Unmarshal(errRaw, &rpcErr); err != nil {
					return nil, fmt.Errorf("%s rpc error decode: %w", method, err)
				}
				return nil, fmt.Errorf("%s failed: code=%d message=%s", method, rpcErr.Code, rpcErr.Message)
			}

			var result map[string]json.RawMessage
			if raw, ok := msg["result"]; ok && string(raw) != "null" {
				if err := json.Unmarshal(raw, &result); err != nil {
					return nil, fmt.Errorf("decode %s result: %w", method, err)
				}
			} else {
				result = map[string]json.RawMessage{}
			}
			return result, nil
		case <-timeout.C:
			return nil, fmt.Errorf("timed out waiting for %s response; stderr=%s", method, stderr())
		}
	}
}

func writeMessage(stdin interface{ Write([]byte) (int, error) }, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = stdin.Write(raw)
	return err
}

func decodeNamedList(raw json.RawMessage, field string) ([]string, error) {
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode %s list: %w", field, err)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out, nil
}

func decodeTemplateList(raw json.RawMessage) ([]string, error) {
	var items []struct {
		URITemplate string `json:"uriTemplate"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode resourceTemplates list: %w", err)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.URITemplate)
	}
	return out, nil
}

func decodeResourceCount(raw json.RawMessage) (int, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("decode resources list: %w", err)
	}
	return len(items), nil
}

#!/usr/bin/env python3
import argparse
import json
import select
import subprocess
import sys
import time


def write_msg(proc, payload):
    line = (json.dumps(payload, separators=(",", ":")) + "\n").encode("utf-8")
    proc.stdin.write(line)
    proc.stdin.flush()


def read_msg(proc, timeout_sec):
    deadline = time.time() + timeout_sec
    while True:
        remaining = deadline - time.time()
        if remaining <= 0:
            raise TimeoutError("timed out waiting for MCP response")
        ready, _, _ = select.select([proc.stdout], [], [], remaining)
        if not ready:
            continue
        line = proc.stdout.readline()
        if not line:
            raise RuntimeError("MCP process closed stdout before response")
        line = line.strip()
        if not line:
            continue
        return json.loads(line.decode("utf-8"))


def rpc(proc, rid, method, params, timeout_sec):
    write_msg(proc, {"jsonrpc": "2.0", "id": rid, "method": method, "params": params})
    while True:
        resp = read_msg(proc, timeout_sec)
        if "id" not in resp:
            continue
        if resp.get("id") != rid:
            continue
        break
    if "error" in resp and resp["error"]:
        raise RuntimeError(f"{method} failed: {resp['error']}")
    return resp.get("result", {})


def main():
    ap = argparse.ArgumentParser(description="Smoke test for confluence-replica MCP server")
    ap.add_argument("--config", default="config/config.yaml", help="Path to config yaml")
    ap.add_argument("--timeout", type=float, default=8.0, help="Timeout per MCP request in seconds")
    ap.add_argument(
        "--command",
        default="./bin/mcp",
        help='MCP executable (default: ./bin/mcp). Use "go run ./cmd/mcp" via --go-run.',
    )
    ap.add_argument("--go-run", action="store_true", help="Use go run ./cmd/mcp instead of --command")
    args = ap.parse_args()

    if args.go_run:
        cmd = ["go", "run", "./cmd/mcp", "--config", args.config]
    else:
        cmd = [args.command, "--config", args.config]

    print(f"[smoke] starting: {' '.join(cmd)}")
    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=False,
    )

    try:
        init = rpc(
            proc,
            1,
            "initialize",
            {
                "protocolVersion": "2025-06-18",
                "capabilities": {},
                "clientInfo": {"name": "mcp-smoke", "version": "1"},
            },
            args.timeout,
        )
        print(f"[ok] initialize: {init.get('serverInfo', {})}")
        write_msg(proc, {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})

        tools = rpc(proc, 2, "tools/list", {}, args.timeout).get("tools", [])
        resources = rpc(proc, 3, "resources/list", {}, args.timeout).get("resources", [])
        templates = rpc(proc, 4, "resources/templates/list", {}, args.timeout).get("resourceTemplates", [])
        prompts = rpc(proc, 5, "prompts/list", {}, args.timeout).get("prompts", [])

        tool_names = {t.get("name", "") for t in tools}
        prompt_names = {p.get("name", "") for p in prompts}
        template_uris = {r.get("uriTemplate", "") for r in templates}

        expected_tools = {"search", "ask", "get_tree"}
        expected_prompts = {"daily_brief", "investigate_page", "compare_versions"}
        expected_templates = {
            "confluence://page/{page_id}",
            "confluence://chunk/{chunk_id}",
            "confluence://digest/{date}",
        }

        missing_tools = expected_tools - tool_names
        missing_prompts = expected_prompts - prompt_names
        missing_templates = expected_templates - template_uris

        print(f"[ok] tools/list: {sorted(tool_names)}")
        print(f"[ok] resources/list: {len(resources)} static resources")
        print(f"[ok] resources/templates/list: {sorted(template_uris)}")
        print(f"[ok] prompts/list: {sorted(prompt_names)}")

        if missing_tools or missing_templates or missing_prompts:
            if missing_tools:
                print(f"[fail] missing tools: {sorted(missing_tools)}", file=sys.stderr)
            if missing_templates:
                print(f"[fail] missing resource templates: {sorted(missing_templates)}", file=sys.stderr)
            if missing_prompts:
                print(f"[fail] missing prompts: {sorted(missing_prompts)}", file=sys.stderr)
            return 2

        print("[pass] MCP smoke test passed")
        return 0
    except Exception as exc:
        err = b""
        try:
            err = proc.stderr.read1(8192)
        except Exception:
            pass
        if err:
            print("[stderr]")
            print(err.decode("utf-8", errors="replace").strip())
        print(f"[fail] {exc}", file=sys.stderr)
        return 1
    finally:
        try:
            proc.terminate()
            proc.wait(timeout=1.5)
        except Exception:
            proc.kill()


if __name__ == "__main__":
    sys.exit(main())

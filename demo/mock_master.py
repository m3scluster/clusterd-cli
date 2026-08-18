#!/usr/bin/env python3
"""Minimal synthetic Mesos master used only by the terminal demo."""

import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse


AGENTS = {
    "slaves": [
        {
            "id": "synthetic-agent-001",
            "hostname": "node-01.example.test",
            "active": True,
            "pid": "slave(1)@127.0.0.1:5051",
        }
    ]
}

FRAMEWORKS = {
    "frameworks": [
        {
            "id": "synthetic-framework-001",
            "active": True,
            "hostname": "scheduler.example.test",
            "name": "Synthetic Scheduler",
            "webui_url": "http://scheduler.example.test",
            "tasks": [],
            "unreachable_tasks": [],
            "completed_tasks": [],
        },
        {
            "id": "synthetic-compose-framework",
            "active": True,
            "hostname": "compose.example.test",
            "name": "synthetic-compose",
            "webui_url": "",
            "tasks": [],
            "unreachable_tasks": [],
            "completed_tasks": [],
        },
        {
            "id": "synthetic-m3s-framework",
            "active": True,
            "hostname": "m3s.example.test",
            "name": "synthetic-m3s",
            "webui_url": "",
            "tasks": [],
            "unreachable_tasks": [],
            "completed_tasks": [],
        },
        {
            "id": "synthetic-framework-archived",
            "active": False,
            "hostname": "archive.example.test",
            "name": "Archived Synthetic Scheduler",
            "tasks": [],
            "unreachable_tasks": [],
            "completed_tasks": [],
        },
    ]
}

TASKS = {
    "tasks": [
        {
            "id": "synthetic-task-running",
            "state": "TASK_RUNNING",
            "framework_id": "synthetic-framework-001",
            "executor_id": "synthetic-executor-001",
            "slave_id": "synthetic-agent-001",
            "statuses": [{"state": "TASK_RUNNING"}],
        },
        {
            "id": "synthetic-task-finished",
            "state": "TASK_FINISHED",
            "framework_id": "synthetic-framework-001",
            "executor_id": "synthetic-executor-001",
            "slave_id": "synthetic-agent-001",
            "statuses": [{"state": "TASK_FINISHED"}],
        },
    ]
}

COMPOSE_TASKS = [
    {
        "TaskID": "synthetic-compose-task",
        "task_name": "demo:web:frontend",
        "State": "TASK_RUNNING",
        "MesosAgent": {"hostname": "node-01.example.test"},
    }
]

M3S_STATUS = {"status": "healthy", "servers": 1, "agents": 2}
KUBERNETES_STATUS = {"status": "ready", "nodes": 3}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = urlparse(self.path).path.rstrip("/")
        payload = {
            "/slaves": AGENTS,
            "/master/frameworks": FRAMEWORKS,
            "/tasks": TASKS,
            "/api/compose/v0/tasks": COMPOSE_TASKS,
            "/api/m3s/v0/status/m3s": M3S_STATUS,
            "/api/m3s/v0/status/k8s": KUBERNETES_STATUS,
        }.get(path)
        if payload is None:
            self.send_error(404)
            return
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        del format, args
        return


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: mock_master.py <port-file>")
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    framework_url = f"http://127.0.0.1:{server.server_port}"
    for framework in FRAMEWORKS["frameworks"]:
        if framework["name"] in {"synthetic-compose", "synthetic-m3s"}:
            framework["webui_url"] = framework_url
    Path(sys.argv[1]).write_text(str(server.server_port), encoding="utf-8")
    server.serve_forever()


if __name__ == "__main__":
    main()

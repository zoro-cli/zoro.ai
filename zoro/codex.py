"""Codex CLI and validation command execution."""

from __future__ import annotations

import shlex
import shutil
import subprocess
import time
from pathlib import Path

from zoro.errors import CodexError
from zoro.models import CodexResult, CommandResult

CODEX_INSTRUCTION = """Implement the attached handoff. Follow repository instructions. Inspect existing code
before editing. Implement only the requested handoff. Do not refactor unrelated code. Preserve user changes.
Run appropriate tests and report affected files and validation."""


def codex_available() -> bool:
    return shutil.which("codex") is not None


def invoke_codex(root: Path, handoff: Path) -> CodexResult:
    if not codex_available():
        raise CodexError("Codex CLI is not installed or is not available on PATH.")
    prompt = f"{CODEX_INSTRUCTION}\n\nHandoff path: {handoff.relative_to(root)}"
    started = time.monotonic()
    try:
        result = subprocess.run(
            ["codex", "exec", "--full-auto", prompt], cwd=root, capture_output=True, text=True,
            encoding="utf-8", errors="replace",
        )
    except (OSError, KeyboardInterrupt) as exc:
        raise CodexError(f"Codex execution failed: {exc}") from exc
    return CodexResult(
        command="codex exec --full-auto <handoff>", exit_code=result.returncode,
        stdout=result.stdout[-20_000:], stderr=result.stderr[-20_000:],
        duration_seconds=time.monotonic() - started,
    )


def run_validation(root: Path, commands: list[str]) -> list[CommandResult]:
    results: list[CommandResult] = []
    for command in commands:
        started = time.monotonic()
        argv = shlex.split(command, posix=False)
        result = subprocess.run(
            argv, cwd=root, capture_output=True, text=True, encoding="utf-8", errors="replace",
        )
        record = CommandResult(
            command=command, exit_code=result.returncode, stdout=result.stdout[-20_000:],
            stderr=result.stderr[-20_000:], duration_seconds=time.monotonic() - started,
        )
        results.append(record)
        if result.returncode:
            break
    return results

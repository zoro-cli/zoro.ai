"""Safe, deterministic Git repository inspection."""

from __future__ import annotations

import fnmatch
import re
import shutil
import subprocess
from pathlib import Path

from zoro.errors import RepositoryError
from zoro.models import RepositoryContext, RepositoryIdentity

EXCLUDED_DIRS = {".git", ".venv", "node_modules", "vendor", "dist", "build", "coverage", "__pycache__"}
SECRET_PATTERNS = (".env", ".env.*", "*.pem", "*.key", "id_rsa", "id_ed25519", "credentials*", "secrets*")
METADATA_PATHS = (
    "AGENTS.md", "CLAUDE.md", "README.md", "pyproject.toml", "package.json", "go.mod",
    "Cargo.toml", "Dockerfile", "docker-compose.yml",
)
MAX_FILE_BYTES = 100_000


def _run(root: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            args, cwd=root, check=check, capture_output=True, text=True, encoding="utf-8",
            errors="replace",
        )
    except (FileNotFoundError, subprocess.CalledProcessError) as exc:
        detail = exc.stderr.strip() if isinstance(exc, subprocess.CalledProcessError) else str(exc)
        raise RepositoryError(f"Git command failed: {detail}") from exc


def ensure_repository(root: Path) -> None:
    result = _run(root, "git", "rev-parse", "--is-inside-work-tree", check=False)
    if result.returncode or result.stdout.strip() != "true":
        raise RepositoryError(f"Not inside a Git repository: {root}")


def repository_root(cwd: Path) -> Path:
    result = _run(cwd, "git", "rev-parse", "--show-toplevel", check=False)
    if result.returncode or not result.stdout.strip():
        raise RepositoryError(
            "Current directory is not a Git repository.\n\nInitialize Git first:\n\n"
            "  git init\n\nor run zoro inside an existing repository."
        )
    return Path(result.stdout.strip()).resolve()


def git_remotes(root: Path) -> dict[str, str]:
    names = _run(root, "git", "remote").stdout.splitlines()
    return {name: _run(root, "git", "remote", "get-url", name).stdout.strip() for name in names}


def parse_github_remote(remote_name: str, remote_url: str, root: Path | None = None) -> RepositoryIdentity:
    value = remote_url.strip()
    patterns = (
        r"^git@github\.com:(?P<owner>[^/]+)/(?P<repo>[^/]+?)(?:\.git)?$",
        r"^https://github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+?)(?:\.git)?/?$",
        r"^ssh://git@github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+?)(?:\.git)?/?$",
    )
    match = next((match for pattern in patterns if (match := re.fullmatch(pattern, value, re.IGNORECASE))), None)
    if not match:
        raise RepositoryError(f"Unsupported Git remote:\n\n  {value or '(empty)'}\n\nzoro.ai currently supports GitHub repositories only.")
    return RepositoryIdentity(
        root=(root or Path.cwd()).resolve(), remote_name=remote_name, remote_url=value,
        owner=match.group("owner"), repo=match.group("repo"),
    )


def resolve_repository_identity(cwd: Path, remote_name: str | None = None) -> RepositoryIdentity:
    root = repository_root(cwd)
    remotes = git_remotes(root)
    if not remotes:
        raise RepositoryError(
            "Git repository detected, but no Git remote exists.\n\nAdd a GitHub remote first:\n\n"
            "  git remote add origin <repository-url>\n\nThen run:\n\n  zoro init"
        )
    selected = remote_name or ("origin" if "origin" in remotes else next(iter(remotes)) if len(remotes) == 1 else None)
    if selected is None:
        raise RepositoryError("Multiple Git remotes exist and no origin is configured: " + ", ".join(remotes))
    if selected not in remotes:
        raise RepositoryError(f"Git remote does not exist: {selected}")
    return parse_github_remote(selected, remotes[selected], root)


def git_status(root: Path) -> str:
    ensure_repository(root)
    return _run(root, "git", "status", "--porcelain").stdout.rstrip()


def ensure_clean(root: Path) -> None:
    status = git_status(root)
    if status:
        raise RepositoryError(
            "Cannot start implementation.\n\nRepository contains uncommitted changes:\n\n"
            f"{status}\n\nCommit, stash, or remove these changes first."
        )


def slugify(value: str, max_length: int = 60) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")
    return slug[:max_length].rstrip("-") or "task"


def branch_name(prefix: str, issue: int | None, title: str) -> str:
    identity = str(issue) if issue is not None else "item"
    return f"{prefix}/{identity}-{slugify(title)}"


def create_branch(root: Path, name: str) -> None:
    existing = _run(root, "git", "branch", "--list", name).stdout.strip()
    if existing:
        raise RepositoryError(f"Branch already exists: {name}")
    _run(root, "git", "switch", "-c", name)


def _safe_file(path: Path, root: Path) -> bool:
    relative = path.relative_to(root)
    if any(part in EXCLUDED_DIRS for part in relative.parts):
        return False
    if any(fnmatch.fnmatch(path.name, pattern) for pattern in SECRET_PATTERNS):
        return False
    try:
        if not path.is_file() or path.stat().st_size > MAX_FILE_BYTES:
            return False
        sample = path.read_bytes()[:4096]
        return b"\x00" not in sample
    except OSError:
        return False


def _tracked_files(root: Path) -> list[Path]:
    result = _run(root, "git", "ls-files", "--cached", "--others", "--exclude-standard")
    return [root / line for line in result.stdout.splitlines() if line]


def _keywords(title: str, body: str) -> list[str]:
    stop = {"the", "and", "for", "with", "that", "this", "from", "into", "add", "fix", "use"}
    words = re.findall(r"[A-Za-z_][A-Za-z0-9_]{2,}", f"{title} {body}")
    return list(dict.fromkeys(word.lower() for word in words if word.lower() not in stop))[:12]


def collect_context(
    root: Path, title: str, body: str, max_files: int = 30, max_bytes: int = 300_000
) -> RepositoryContext:
    ensure_repository(root)
    candidates = [path for path in _tracked_files(root) if _safe_file(path, root)]
    selected: list[Path] = []
    for relative in METADATA_PATHS:
        path = root / relative
        if path in candidates:
            selected.append(path)
    for path in candidates:
        relative = path.relative_to(root).as_posix()
        if relative.startswith((".github/", "docs/")) and path not in selected:
            selected.append(path)
    keywords = _keywords(title, body)
    scored: list[tuple[int, str, Path]] = []
    for path in candidates:
        if path in selected:
            continue
        relative = path.relative_to(root).as_posix().lower()
        score = sum(3 for key in keywords if key in relative)
        try:
            content = path.read_text(encoding="utf-8", errors="replace").lower()
            score += sum(1 for key in keywords if key in content)
        except OSError:
            continue
        if score or "/test" in relative or relative.startswith("test"):
            scored.append((-score, relative, path))
    selected.extend(item[2] for item in sorted(scored))
    files: dict[str, str] = {}
    used = 0
    for path in selected:
        if len(files) >= max_files:
            break
        text = path.read_text(encoding="utf-8", errors="replace")
        size = len(text.encode("utf-8"))
        if used + size > max_bytes:
            continue
        files[path.relative_to(root).as_posix()] = text
        used += size
    tree = [path.relative_to(root).as_posix() for path in candidates][:500]
    return RepositoryContext(root=root, tree=tree, files=files, git_status=git_status(root))


def executable_exists(name: str) -> bool:
    return shutil.which(name) is not None

import subprocess
from pathlib import Path

import pytest

from zoro.errors import RepositoryError
from zoro.repository import branch_name, collect_context, ensure_clean, git_status


def git(root: Path, *args: str) -> None:
    subprocess.run(["git", *args], cwd=root, check=True, capture_output=True)


@pytest.fixture
def repo(tmp_path: Path) -> Path:
    git(tmp_path, "init")
    git(tmp_path, "config", "user.email", "test@example.com")
    git(tmp_path, "config", "user.name", "Test")
    (tmp_path / "README.md").write_text("refresh token service", encoding="utf-8")
    (tmp_path / ".env").write_text("SECRET=x", encoding="utf-8")
    (tmp_path / ".gitignore").write_text(".env\n", encoding="utf-8")
    (tmp_path / "src").mkdir()
    (tmp_path / "src/auth.py").write_text("def refresh_token(): pass", encoding="utf-8")
    git(tmp_path, "add", "README.md", "src/auth.py", ".gitignore")
    git(tmp_path, "commit", "-m", "initial")
    return tmp_path


def test_clean_and_dirty_detection(repo: Path) -> None:
    assert git_status(repo) == ""
    (repo / "src/auth.py").write_text("changed", encoding="utf-8")
    with pytest.raises(RepositoryError, match="uncommitted changes"):
        ensure_clean(repo)


def test_branch_name() -> None:
    assert branch_name("zoro", 142, "Add Refresh Token Rotation!") == "zoro/142-add-refresh-token-rotation"


def test_context_is_bounded_and_excludes_secrets(repo: Path) -> None:
    context = collect_context(repo, "Refresh token", "rotation", max_files=5, max_bytes=10_000)
    assert "src/auth.py" in context.files
    assert ".env" not in context.files

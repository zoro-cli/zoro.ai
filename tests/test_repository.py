import subprocess
from pathlib import Path

import pytest

from zoro.errors import RepositoryError
from zoro.repository import (
    branch_name,
    collect_context,
    ensure_clean,
    git_status,
    parse_github_remote,
    resolve_repository_identity,
)


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


@pytest.mark.parametrize(("remote", "owner", "name"), [
    ("git@github.com:nenjotech/zoro-ai.git", "nenjotech", "zoro-ai"),
    ("https://github.com/nenjotech/zoro-ai.git", "nenjotech", "zoro-ai"),
    ("https://github.com/acme/platform", "acme", "platform"),
    ("ssh://git@github.com/acme/platform.git", "acme", "platform"),
])
def test_parse_github_remote(remote: str, owner: str, name: str) -> None:
    result = parse_github_remote("origin", remote)
    assert (result.owner, result.repo) == (owner, name)


def test_nested_repository_resolution(repo: Path) -> None:
    git(repo, "remote", "add", "origin", "git@github.com:acme/platform.git")
    nested = repo / "src/nested"
    nested.mkdir()
    result = resolve_repository_identity(nested)
    assert result.root == repo.resolve()
    assert (result.owner, result.repo) == ("acme", "platform")


@pytest.mark.parametrize("remote", ["", "git@gitlab.com:a/b.git", "https://bitbucket.org/a/b.git", "bad"])
def test_invalid_remote(remote: str) -> None:
    with pytest.raises(RepositoryError, match="Unsupported Git remote"):
        parse_github_remote("origin", remote)

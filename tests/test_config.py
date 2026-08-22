from pathlib import Path

import pytest
import yaml
from pydantic import ValidationError

from zoro.config import GitHubConfig, ZoroConfig, has_valid_repository, initialize, load_config, parse_duration
from zoro.errors import ConfigError


def test_valid_config_and_initialize(tmp_path: Path) -> None:
    (tmp_path / ".gitignore").write_text("", encoding="utf-8")
    initialize(tmp_path, ZoroConfig(github=GitHubConfig(owner="acme", repo="app")))
    config = load_config(tmp_path)
    assert config.github.project_number == 1
    assert (tmp_path / "handoff/ready").is_dir()
    assert ".zoro/runtime/" in (tmp_path / ".gitignore").read_text(encoding="utf-8")


def test_placeholder_repository_is_incomplete() -> None:
    assert not has_valid_repository(GitHubConfig())
    assert not has_valid_repository(GitHubConfig(owner="", repo=""))
    assert has_valid_repository(GitHubConfig(owner="acme", repo="app"))


@pytest.mark.parametrize("value", ["", "1d", "0s", "soon", "1.5m"])
def test_invalid_duration(value: str) -> None:
    with pytest.raises(ValueError):
        parse_duration(value)


def test_invalid_project_number() -> None:
    with pytest.raises(ValidationError):
        ZoroConfig.model_validate({"github": {"project_number": 0}})


def test_missing_status_mapping(tmp_path: Path) -> None:
    path = tmp_path / ".zoro/config.yaml"
    path.parent.mkdir()
    path.write_text(yaml.safe_dump({"github": {"statuses": {"ready": "Ready"}}}), encoding="utf-8")
    with pytest.raises(ConfigError):
        load_config(tmp_path)

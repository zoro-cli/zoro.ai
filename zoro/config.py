"""Configuration loading, validation, and initialization."""

from __future__ import annotations

import re
from pathlib import Path

import yaml
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from zoro.errors import ConfigError

CONFIG_PATH = Path(".zoro/config.yaml")
HANDOFF_STATES = ("ready", "implementing", "review", "done", "failed")
PLACEHOLDER_VALUES = {"", "owner", "repository", "<owner>", "<repository>"}


def parse_duration(value: str) -> int:
    match = re.fullmatch(r"([1-9]\d*)(s|m|h)", value.strip())
    if not match:
        raise ValueError("duration must use a positive integer followed by s, m, or h")
    amount, unit = int(match.group(1)), match.group(2)
    return amount * {"s": 1, "m": 60, "h": 3600}[unit]


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class Statuses(StrictModel):
    backlog: str
    ready: str
    implementing: str
    review: str
    done: str

    @model_validator(mode="after")
    def unique_values(self) -> Statuses:
        if len(set(self.model_dump().values())) != 5:
            raise ValueError("project status names must be unique")
        return self


class GitHubConfig(StrictModel):
    owner: str = "OWNER"
    repo: str = "REPOSITORY"
    project_number: int = Field(default=1, gt=0)
    status_field: str = "Status"
    statuses: Statuses = Field(
        default_factory=lambda: Statuses(
            backlog="Backlog", ready="Ready", implementing="In progress",
            review="In review", done="Done"
        )
    )


def has_valid_repository(config: GitHubConfig) -> bool:
    return config.owner.strip().lower() not in PLACEHOLDER_VALUES and config.repo.strip().lower() not in PLACEHOLDER_VALUES


class SchedulerConfig(StrictModel):
    enabled: bool = True
    interval: str = "1m"

    @field_validator("interval")
    @classmethod
    def valid_interval(cls, value: str) -> str:
        parse_duration(value)
        return value


class PlanningConfig(StrictModel):
    provider: str = "openai"
    model: str = "gpt-5.6"
    max_files: int = Field(default=30, ge=1, le=200)
    max_context_bytes: int = Field(default=300_000, ge=10_000, le=2_000_000)


class AutomationConfig(StrictModel):
    auto_plan: bool = True
    auto_implement: bool = False


class BranchConfig(StrictModel):
    enabled: bool = True
    prefix: str = "zoro"

    @field_validator("prefix")
    @classmethod
    def safe_prefix(cls, value: str) -> str:
        if not re.fullmatch(r"[A-Za-z0-9._-]+", value):
            raise ValueError("branch prefix contains unsafe characters")
        return value


class ValidationConfig(StrictModel):
    enabled: bool = True
    commands: list[str] = Field(default_factory=lambda: ["pytest", "ruff check ."])


class ImplementationConfig(StrictModel):
    provider: str = "codex"
    branch: BranchConfig = Field(default_factory=BranchConfig)
    validation: ValidationConfig = Field(default_factory=ValidationConfig)


class HandoffConfig(StrictModel):
    directory: str = "handoff"

    @field_validator("directory")
    @classmethod
    def safe_directory(cls, value: str) -> str:
        path = Path(value)
        if path.is_absolute() or ".." in path.parts:
            raise ValueError("handoff directory must be a safe relative path")
        return value


class BehaviorConfig(StrictModel):
    max_concurrent_tasks: int = Field(default=1, ge=1, le=16)
    move_to_in_progress_on_implement: bool = True
    move_to_review_on_success: bool = True


class ZoroConfig(StrictModel):
    version: int = 1
    github: GitHubConfig = Field(default_factory=GitHubConfig)
    scheduler: SchedulerConfig = Field(default_factory=SchedulerConfig)
    planning: PlanningConfig = Field(default_factory=PlanningConfig)
    automation: AutomationConfig = Field(default_factory=AutomationConfig)
    implementation: ImplementationConfig = Field(default_factory=ImplementationConfig)
    handoff: HandoffConfig = Field(default_factory=HandoffConfig)
    behavior: BehaviorConfig = Field(default_factory=BehaviorConfig)

    @field_validator("version")
    @classmethod
    def supported_version(cls, value: int) -> int:
        if value != 1:
            raise ValueError("only configuration version 1 is supported")
        return value


def load_config(root: Path | None = None) -> ZoroConfig:
    root = root or Path.cwd()
    path = root / CONFIG_PATH
    if not path.exists():
        raise ConfigError(f"Configuration not found at {path}. Run 'zoro init' first.")
    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
        return ZoroConfig.model_validate(data)
    except (OSError, yaml.YAMLError, ValueError) as exc:
        raise ConfigError(f"Invalid configuration {path}: {exc}") from exc


def initialize(root: Path | None = None, config: ZoroConfig | None = None, overwrite: bool = False) -> list[Path]:
    root = root or Path.cwd()
    created: list[Path] = []
    config_path = root / CONFIG_PATH
    config_path.parent.mkdir(parents=True, exist_ok=True)
    if not config_path.exists() or overwrite:
        if config is None:
            raise ConfigError("Initialization requires a validated configuration.")
        temporary = config_path.with_suffix(".yaml.tmp")
        temporary.write_text(yaml.safe_dump(config.model_dump(), sort_keys=False), encoding="utf-8")
        temporary.replace(config_path)
        created.append(config_path)
    handoff_root = root / "handoff"
    for state in HANDOFF_STATES:
        directory = handoff_root / state
        if not directory.exists():
            directory.mkdir(parents=True)
            created.append(directory)
    runtime = root / ".zoro/runtime"
    if not runtime.exists():
        runtime.mkdir(parents=True)
        created.append(runtime)
    gitignore = root / ".gitignore"
    current = gitignore.read_text(encoding="utf-8") if gitignore.exists() else ""
    if ".zoro/runtime/" not in current.splitlines():
        with gitignore.open("a", encoding="utf-8") as stream:
            if current and not current.endswith("\n"):
                stream.write("\n")
            stream.write(".zoro/runtime/\n")
        created.append(gitignore)
    return created

"""Shared data models."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path

from pydantic import BaseModel, Field


class ProjectItem(BaseModel):
    id: str
    content_id: str | None = None
    issue_number: int | None = None
    title: str
    body: str = ""
    status: str | None = None
    position: int = 0
    repository: str | None = None


class ProjectInfo(BaseModel):
    id: str
    title: str
    status_field_id: str
    status_options: dict[str, str]
    items: list[ProjectItem] = Field(default_factory=list)


class RelevantFile(BaseModel):
    path: str
    reason: str
    expected_change: str | None = None


class ProposedChange(BaseModel):
    file: str | None = None
    description: str
    risk: str | None = None


class AcceptanceCriterion(BaseModel):
    criterion: str
    validation: str | None = None


class HandoffPlan(BaseModel):
    summary: str
    objective: str
    assumptions: list[str] = Field(default_factory=list)
    relevant_files: list[RelevantFile] = Field(default_factory=list)
    proposed_changes: list[ProposedChange] = Field(default_factory=list)
    preparation: list[str] = Field(default_factory=list)
    implementation_steps: list[str] = Field(default_factory=list)
    validation_steps: list[str] = Field(default_factory=list)
    risks: list[str] = Field(default_factory=list)
    acceptance_criteria: list[AcceptanceCriterion] = Field(default_factory=list)


class RepositoryContext(BaseModel):
    root: Path
    tree: list[str]
    files: dict[str, str]
    git_status: str = ""


class CommandResult(BaseModel):
    command: str
    exit_code: int
    stdout: str = ""
    stderr: str = ""
    duration_seconds: float = 0


class CodexResult(CommandResult):
    pass


class HandoffMetadata(BaseModel):
    zoro_version: str
    issue: int | None
    repository: str
    project_item_id: str
    status: str
    generated_at: datetime
    planner: str
    model: str

"""Deterministic handoff rendering and directory state management."""

from __future__ import annotations

import re
import shutil
from datetime import datetime
from pathlib import Path

import yaml

from zoro import __version__
from zoro.errors import HandoffError
from zoro.models import HandoffMetadata, HandoffPlan, ProjectItem
from zoro.repository import slugify

STATES = ("ready", "implementing", "review", "done", "failed")


def handoff_filename(item: ProjectItem) -> str:
    identity = str(item.issue_number) if item.issue_number is not None else item.id.replace("_", "-")
    return f"{identity}-{slugify(item.title)}.md"


def render_handoff(
    item: ProjectItem, plan: HandoffPlan, repository: str, model: str,
    generated_at: datetime | None = None, dirty: bool = False,
) -> str:
    metadata = HandoffMetadata(
        zoro_version=__version__, issue=item.issue_number, repository=repository,
        project_item_id=item.id, status="ready", generated_at=generated_at or datetime.now().astimezone(),
        planner="openai", model=model,
    )
    frontmatter = yaml.safe_dump(metadata.model_dump(mode="json"), sort_keys=False).strip()

    def bullets(values: list[str], empty: str = "None identified.") -> str:
        return "\n".join(f"- {value}" for value in values) if values else empty

    criteria = "\n".join(f"- [ ] {c.criterion}" for c in plan.acceptance_criteria)
    if not criteria:
        criteria = "No explicit acceptance criteria were provided in the issue."
    relevant = "\n".join(
        f"- `{f.path}` — {f.reason}" + (f" Expected change: {f.expected_change}" if f.expected_change else "")
        for f in plan.relevant_files
    ) or "No relevant tracked files were identified from the bounded repository context."
    changes = "\n".join(
        f"- {f'`{c.file}` — ' if c.file else ''}{c.description}" + (f" Risk: {c.risk}" if c.risk else "")
        for c in plan.proposed_changes
    ) or "No file changes proposed."
    def numbered(values: list[str]) -> str:
        return "\n".join(f"{i}. {v}" for i, v in enumerate(values, 1)) or "None specified."
    constraints = [
        "Follow repository instructions and inspect existing code before editing.",
        "Implement only this handoff; do not refactor unrelated code.",
        "Preserve user changes and never expose credentials.",
    ]
    if dirty:
        constraints.append("The repository was dirty during planning; implementation must wait for a clean tree.")
    return f"""---
{frontmatter}
---

# {item.title}

## Objective

{plan.objective}

## Issue Context

{item.body or 'No issue body was provided.'}

{plan.summary}

## Acceptance Criteria

{criteria}

## Repository Analysis

### Relevant Files

{relevant}

Assumptions:

{bullets(plan.assumptions)}

## Preparation

{bullets(plan.preparation)}

## Proposed Changes

{changes}

## Implementation Plan

{numbered(plan.implementation_steps)}

## Validation

{bullets(plan.validation_steps, 'No validation steps were proposed.')}

## Risks

{bullets(plan.risks)}

## Implementation Constraints

{bullets(constraints)}

## Definition of Done

{criteria}
"""


class HandoffStore:
    def __init__(self, root: Path, directory: str = "handoff"):
        self.root = root
        self.base = root / directory

    def ensure(self) -> None:
        for state in STATES:
            (self.base / state).mkdir(parents=True, exist_ok=True)

    def find(self, issue: int | None = None, item_id: str | None = None) -> Path | None:
        for state in STATES:
            for path in sorted((self.base / state).glob("*.md")):
                if issue is not None and re.match(rf"^{issue}-", path.name):
                    return path
                if item_id and f"project_item_id: {item_id}" in path.read_text(encoding="utf-8"):
                    return path
        return None

    def ready(self) -> list[Path]:
        return sorted((self.base / "ready").glob("*.md"))

    def write(self, item: ProjectItem, content: str) -> Path:
        self.ensure()
        if existing := self.find(item.issue_number, item.id):
            raise HandoffError(f"Handoff already exists: {existing.relative_to(self.root)}")
        target = self.base / "ready" / handoff_filename(item)
        target.write_text(content, encoding="utf-8")
        return target

    def move(self, path: Path, state: str) -> Path:
        if state not in STATES:
            raise HandoffError(f"Unknown handoff state: {state}")
        target = self.base / state / path.name
        if target.exists():
            raise HandoffError(f"Target handoff already exists: {target}")
        target.parent.mkdir(parents=True, exist_ok=True)
        try:
            return Path(shutil.move(str(path), target))
        except OSError as exc:
            raise HandoffError(f"Could not move handoff to {state}: {exc}") from exc


def parse_frontmatter(path: Path) -> dict:
    text = path.read_text(encoding="utf-8")
    match = re.match(r"^---\s*\n(.*?)\n---", text, re.DOTALL)
    if not match:
        raise HandoffError(f"Invalid handoff frontmatter: {path}")
    data = yaml.safe_load(match.group(1))
    if not isinstance(data, dict) or not data.get("project_item_id"):
        raise HandoffError(f"Incomplete handoff frontmatter: {path}")
    return data

"""Planning and implementation orchestration."""

from __future__ import annotations

import time
from collections.abc import Callable
from pathlib import Path

from filelock import FileLock, Timeout

from zoro.codex import invoke_codex, run_validation
from zoro.config import ZoroConfig, parse_duration
from zoro.errors import CodexError, LockError, ValidationError
from zoro.github import GitHubClient
from zoro.handoff import HandoffStore, parse_frontmatter, render_handoff
from zoro.models import ProjectInfo, ProjectItem
from zoro.planner import Planner
from zoro.repository import branch_name, collect_context, create_branch, ensure_clean


class Runner:
    def __init__(
        self, root: Path, config: ZoroConfig, github: GitHubClient | None = None,
        planner: Planner | None = None,
    ):
        self.root = root
        self.config = config
        self.github = github or GitHubClient(config.github)
        self.planner = planner
        self.store = HandoffStore(root, config.handoff.directory)
        self.lock = FileLock(root / ".zoro/runtime/zoro.lock")

    def _planner(self) -> Planner:
        if self.planner is None:
            self.planner = Planner(self.config.planning.model)
        return self.planner

    def select_item(self, issue: int | None = None) -> tuple[ProjectInfo, ProjectItem]:
        project = self.github.get_project()
        candidates = project.items if issue is not None else self.github.ready_items(project)
        item = next((candidate for candidate in candidates if candidate.issue_number == issue), None) if issue is not None else (candidates[0] if candidates else None)
        if item is None:
            label = f"issue #{issue}" if issue is not None else "Ready items"
            raise LookupError(f"No {label} found.")
        return project, item

    def plan_item(self, item: ProjectItem) -> Path:
        if existing := self.store.find(item.issue_number, item.id):
            return existing
        context = collect_context(
            self.root, item.title, item.body, self.config.planning.max_files,
            self.config.planning.max_context_bytes,
        )
        plan = self._planner().plan(item, context)
        repository = item.repository or f"{self.config.github.owner}/{self.config.github.repo}"
        rendered = render_handoff(
            item, plan, repository, self.config.planning.model, dirty=bool(context.git_status),
        )
        return self.store.write(item, rendered)

    def run_once(self) -> Path | None:
        try:
            with self.lock.acquire(timeout=0):
                project = self.github.get_project()
                ready = self.github.ready_items(project)
                if not ready or not self.config.automation.auto_plan:
                    return None
                item = ready[0]
                if self.store.find(item.issue_number, item.id):
                    return None
                handoff = self.plan_item(item)
                if self.config.automation.auto_implement:
                    self._implement_locked(handoff, project, item)
                return handoff
        except Timeout as exc:
            raise LockError("Another Zoro process is active for this repository.") from exc

    def run_forever(self, on_cycle: Callable[[Path | None], None] | None = None) -> None:
        interval = parse_duration(self.config.scheduler.interval)
        while True:
            result = self.run_once()
            if on_cycle:
                on_cycle(result)
            time.sleep(interval)

    def implement(self, handoff: Path) -> Path:
        try:
            with self.lock.acquire(timeout=0):
                metadata = parse_frontmatter(handoff)
                project = self.github.get_project()
                item = next((item for item in project.items if item.id == metadata["project_item_id"]), None)
                if item is None:
                    raise LookupError(f"Project item no longer exists: {metadata['project_item_id']}")
                return self._implement_locked(handoff, project, item)
        except Timeout as exc:
            raise LockError("Another Zoro implementation or automatic cycle is active.") from exc

    def _implement_locked(self, handoff: Path, project: ProjectInfo, item: ProjectItem) -> Path:
        ensure_clean(self.root)
        if self.config.implementation.branch.enabled:
            name = branch_name(
                self.config.implementation.branch.prefix, item.issue_number, item.title
            )
            create_branch(self.root, name)
        implementing = self.store.move(handoff, "implementing")
        try:
            if self.config.behavior.move_to_in_progress_on_implement:
                self.github.update_status(project, item, self.config.github.statuses.implementing)
            result = invoke_codex(self.root, implementing)
            if result.exit_code:
                raise CodexError(f"Codex exited with code {result.exit_code}: {result.stderr[-1000:]}")
            commands = (
                self.config.implementation.validation.commands
                if self.config.implementation.validation.enabled else []
            )
            validations = run_validation(self.root, commands)
            failed = next((record for record in validations if record.exit_code), None)
            if failed:
                raise ValidationError(
                    f"Validation failed ({failed.command}, exit {failed.exit_code}): {failed.stderr[-1000:]}"
                )
            reviewed = self.store.move(implementing, "review")
            if self.config.behavior.move_to_review_on_success:
                self.github.update_status(project, item, self.config.github.statuses.review)
            return reviewed
        except Exception as exc:
            if implementing.exists():
                with implementing.open("a", encoding="utf-8") as stream:
                    stream.write(f"\n\n## Implementation Failure\n\n{type(exc).__name__}: {exc}\n")
                self.store.move(implementing, "failed")
            raise

from datetime import UTC, datetime
from pathlib import Path

import pytest

from zoro.errors import HandoffError
from zoro.handoff import HandoffStore, handoff_filename, render_handoff
from zoro.models import AcceptanceCriterion, HandoffPlan, ProjectItem, RelevantFile


@pytest.fixture
def item() -> ProjectItem:
    return ProjectItem(id="PVTI_1", issue_number=142, title="Add refresh token rotation", body="Rotate tokens.")


@pytest.fixture
def plan() -> HandoffPlan:
    return HandoffPlan(
        summary="Add safe rotation.", objective="Rotate refresh tokens after use.",
        relevant_files=[RelevantFile(path="app/auth.py", reason="Authentication logic")],
        implementation_steps=["Update rotation logic", "Add tests"],
        validation_steps=["Run pytest"], risks=["Concurrency"],
        acceptance_criteria=[AcceptanceCriterion(criterion="Old tokens become invalid")],
    )


def test_deterministic_filename(item: ProjectItem) -> None:
    assert handoff_filename(item) == "142-add-refresh-token-rotation.md"


def test_render_required_sections(item: ProjectItem, plan: HandoffPlan) -> None:
    text = render_handoff(item, plan, "owner/repo", "test-model", datetime(2026, 8, 22, tzinfo=UTC))
    assert "project_item_id: PVTI_1" in text
    assert "## Implementation Plan" in text
    assert "- [ ] Old tokens become invalid" in text


def test_duplicate_detection(tmp_path: Path, item: ProjectItem, plan: HandoffPlan) -> None:
    store = HandoffStore(tmp_path)
    content = render_handoff(item, plan, "owner/repo", "model")
    store.write(item, content)
    with pytest.raises(HandoffError, match="already exists"):
        store.write(item, content)

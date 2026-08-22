from pathlib import Path
from unittest.mock import Mock

from zoro.config import ZoroConfig
from zoro.models import HandoffPlan, ProjectInfo, ProjectItem
from zoro.runner import Runner


def test_run_once_selects_top_and_skips_duplicate(tmp_path: Path, monkeypatch) -> None:
    (tmp_path / ".zoro/runtime").mkdir(parents=True)
    item = ProjectItem(id="I1", issue_number=1, title="Top", status="Ready")
    project = ProjectInfo(id="P", title="P", status_field_id="F", status_options={}, items=[item])
    github = Mock()
    github.get_project.return_value = project
    github.ready_items.return_value = [item]
    planner = Mock()
    planner.plan.return_value = HandoffPlan(summary="s", objective="o")
    monkeypatch.setattr("zoro.runner.collect_context", lambda *a, **k: Mock(git_status="", files={}, tree=[]))
    runner = Runner(tmp_path, ZoroConfig(), github=github, planner=planner)
    first = runner.run_once()
    second = runner.run_once()
    assert first and first.exists()
    assert second is None
    planner.plan.assert_called_once()


def test_no_ready_items(tmp_path: Path) -> None:
    (tmp_path / ".zoro/runtime").mkdir(parents=True)
    github = Mock()
    github.get_project.return_value = Mock()
    github.ready_items.return_value = []
    runner = Runner(tmp_path, ZoroConfig(), github=github, planner=Mock())
    assert runner.run_once() is None

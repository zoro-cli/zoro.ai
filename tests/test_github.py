import httpx
import pytest

from zoro.config import GitHubConfig
from zoro.errors import AuthError, GitHubError, ProjectError
from zoro.github import GitHubClient


def payload(options=None):
    options = options or ["Backlog", "Ready", "In progress", "In review", "Done"]
    return {"data": {"organization": {"projectV2": {
        "id": "PVT_1", "title": "Roadmap",
        "fields": {"nodes": [{"id": "F_1", "name": "Status", "options": [{"id": f"O{i}", "name": name} for i, name in enumerate(options)]}]},
        "items": {"pageInfo": {"hasNextPage": False, "endCursor": None}, "nodes": [
            {"id": "I2", "fieldValues": {"nodes": [{"name": "Ready", "field": {"name": "Status"}}]}, "content": {"id": "C2", "number": 2, "title": "Second", "body": "", "repository": {"nameWithOwner": "o/r"}}},
            {"id": "I1", "fieldValues": {"nodes": [{"name": "Ready", "field": {"name": "Status"}}]}, "content": {"id": "C1", "number": 1, "title": "First", "body": "", "repository": {"nameWithOwner": "o/r"}}},
        ]},
    }}, "user": None}}


def test_ready_order_and_status_update() -> None:
    requests = []
    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        body = request.read().decode()
        return httpx.Response(200, json={"data": {"updateProjectV2ItemFieldValue": {"projectV2Item": {"id": "I2"}}}} if "mutation" in body else payload())
    client = GitHubClient(GitHubConfig(owner="o", repo="r", project_number=1), token="x", transport=httpx.MockTransport(handler))
    project = client.get_project()
    assert [item.issue_number for item in client.ready_items(project)] == [2, 1]
    client.update_status(project, project.items[0], "In progress")
    assert len(requests) == 2


def test_missing_status_value() -> None:
    transport = httpx.MockTransport(lambda request: httpx.Response(200, json=payload(["Ready"])))
    client = GitHubClient(GitHubConfig(owner="o", repo="r", project_number=1), token="x", transport=transport)
    with pytest.raises(ProjectError, match="Required status"):
        client.get_project()


def test_auth_failure() -> None:
    transport = httpx.MockTransport(lambda request: httpx.Response(401, json={}))
    client = GitHubClient(GitHubConfig(), token="x", transport=transport)
    with pytest.raises(AuthError):
        client.get_project()


def test_repository_404_does_not_claim_nonexistence() -> None:
    transport = httpx.MockTransport(lambda request: httpx.Response(404, json={}))
    client = GitHubClient(GitHubConfig(owner="o", repo="private"), token="x", transport=transport)
    with pytest.raises(GitHubError, match="may not exist"):
        client.verify_repository()

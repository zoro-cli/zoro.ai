"""Minimal GitHub GraphQL client for Projects v2."""

from __future__ import annotations

import os
import subprocess
from collections import Counter

import httpx

from zoro.config import GitHubConfig
from zoro.errors import AuthError, GitHubError, ProjectError
from zoro.models import ProjectInfo, ProjectItem

GRAPHQL_URL = "https://api.github.com/graphql"
PROJECT_QUERY = """
query($owner: String!, $number: Int!, $cursor: String) {
  organization(login: $owner) { projectV2(number: $number) { ...ProjectData } }
  user(login: $owner) { projectV2(number: $number) { ...ProjectData } }
}
fragment ProjectData on ProjectV2 {
  id title
  fields(first: 100) { nodes {
    ... on ProjectV2SingleSelectField { id name options { id name } }
  } }
  items(first: 100, after: $cursor) { pageInfo { hasNextPage endCursor } nodes {
    id
    fieldValues(first: 20) { nodes {
      ... on ProjectV2ItemFieldSingleSelectValue { name field { ... on ProjectV2FieldCommon { name } } }
    } }
    content {
      ... on Issue { id number title body repository { nameWithOwner } }
      ... on PullRequest { id number title body repository { nameWithOwner } }
      ... on DraftIssue { title body }
    }
  } }
}
"""

UPDATE_STATUS_MUTATION = """
mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $project, itemId: $item, fieldId: $field,
    value: {singleSelectOptionId: $option}
  }) { projectV2Item { id } }
}
"""


def resolve_token() -> str:
    for name in ("ZORO_GITHUB_TOKEN", "GH_TOKEN"):
        if token := os.getenv(name):
            return token.strip()
    try:
        result = subprocess.run(
            ["gh", "auth", "token"], check=True, capture_output=True, text=True, timeout=10
        )
        if result.stdout.strip():
            return result.stdout.strip()
    except (FileNotFoundError, subprocess.SubprocessError):
        pass
    raise AuthError("No GitHub credentials found. Set ZORO_GITHUB_TOKEN, GH_TOKEN, or run 'gh auth login'.")


class GitHubClient:
    def __init__(self, config: GitHubConfig, token: str | None = None, transport=None):
        self.config = config
        self._token = token or resolve_token()
        self._client = httpx.Client(
            base_url="https://api.github.com", transport=transport, timeout=30,
            headers={"Authorization": f"Bearer {self._token}", "Accept": "application/vnd.github+json"},
        )

    def close(self) -> None:
        self._client.close()

    def _graphql(self, query: str, variables: dict) -> dict:
        try:
            response = self._client.post("/graphql", json={"query": query, "variables": variables})
        except httpx.HTTPError as exc:
            raise GitHubError(f"GitHub request failed: {exc}") from exc
        if response.status_code in (401, 403):
            raise AuthError(f"GitHub authentication failed ({response.status_code}).")
        if response.is_error:
            raise GitHubError(f"GitHub returned HTTP {response.status_code}: {response.text[:500]}")
        payload = response.json()
        if errors := payload.get("errors"):
            raise ProjectError("GitHub GraphQL error: " + "; ".join(e.get("message", str(e)) for e in errors))
        return payload["data"]

    def verify_repository(self) -> None:
        response = self._client.get(f"/repos/{self.config.owner}/{self.config.repo}")
        if response.status_code in (401, 403):
            raise AuthError("GitHub authentication failed or token lacks repository access.")
        if response.status_code == 404:
            raise GitHubError(f"Repository not found: {self.config.owner}/{self.config.repo}")
        response.raise_for_status()

    def get_project(self) -> ProjectInfo:
        nodes: list[dict] = []
        cursor = None
        raw_project = None
        while True:
            data = self._graphql(
                PROJECT_QUERY,
                {"owner": self.config.owner, "number": self.config.project_number, "cursor": cursor},
            )
            project = (data.get("organization") or {}).get("projectV2") or (data.get("user") or {}).get("projectV2")
            if not project:
                raise ProjectError(f"Project {self.config.owner} #{self.config.project_number} was not found.")
            raw_project = project
            items = project["items"]
            nodes.extend(items.get("nodes") or [])
            if not items["pageInfo"]["hasNextPage"]:
                break
            cursor = items["pageInfo"]["endCursor"]
        fields = raw_project["fields"]["nodes"]
        status_field = next((f for f in fields if f and f.get("name") == self.config.status_field and "options" in f), None)
        if not status_field:
            raise ProjectError(f'Project status field "{self.config.status_field}" was not found.')
        options = {option["name"]: option["id"] for option in status_field["options"]}
        required = list(self.config.statuses.model_dump().values())
        missing = [name for name in required if name not in options]
        if missing:
            raise ProjectError(
                f"Required status values do not exist: {', '.join(missing)}. "
                f"Available values: {', '.join(options)}"
            )
        parsed: list[ProjectItem] = []
        for position, node in enumerate(nodes):
            content = node.get("content") or {}
            if not content.get("title"):
                continue
            status = None
            for value in node.get("fieldValues", {}).get("nodes", []):
                if value and value.get("field", {}).get("name") == self.config.status_field:
                    status = value.get("name")
                    break
            parsed.append(ProjectItem(
                id=node["id"], content_id=content.get("id"), issue_number=content.get("number"),
                title=content["title"], body=content.get("body") or "", status=status,
                position=position, repository=(content.get("repository") or {}).get("nameWithOwner"),
            ))
        return ProjectInfo(
            id=raw_project["id"], title=raw_project["title"], status_field_id=status_field["id"],
            status_options=options, items=parsed,
        )

    def ready_items(self, project: ProjectInfo | None = None) -> list[ProjectItem]:
        project = project or self.get_project()
        return [item for item in project.items if item.status == self.config.statuses.ready]

    def board_counts(self, project: ProjectInfo | None = None) -> Counter[str]:
        project = project or self.get_project()
        return Counter(item.status for item in project.items if item.status)

    def update_status(self, project: ProjectInfo, item: ProjectItem, status_name: str) -> None:
        option_id = project.status_options.get(status_name)
        if not option_id:
            raise ProjectError(f"Unknown project status: {status_name}")
        self._graphql(UPDATE_STATUS_MUTATION, {
            "project": project.id, "item": item.id, "field": project.status_field_id,
            "option": option_id,
        })

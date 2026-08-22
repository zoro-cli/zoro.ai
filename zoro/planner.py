"""Read-only OpenAI planning with structured output."""

from __future__ import annotations

import json
import os

from openai import OpenAI
from tenacity import retry, retry_if_exception_type, stop_after_attempt, wait_exponential

from zoro.errors import PlannerError
from zoro.models import HandoffPlan, ProjectItem, RepositoryContext

SYSTEM_PROMPT = """You are a read-only software planning agent. Analyze the requested work and supplied
repository context. Do not modify files. Produce a precise implementation plan grounded only in the issue
and context. Do not fabricate acceptance criteria; leave acceptance_criteria empty when none are explicit.
Keep changes scoped and identify concrete validation."""


def build_planning_input(item: ProjectItem, context: RepositoryContext) -> str:
    files = "\n\n".join(f"### {path}\n```\n{content}\n```" for path, content in context.files.items())
    return f"""Project item ID: {item.id}
Issue: {item.issue_number or 'draft item'}
Title: {item.title}
Body:
{item.body or '(empty)'}

Git status:
{context.git_status or '(clean)'}

Repository tree:
{json.dumps(context.tree, indent=2)}

Relevant repository files:
{files or '(none found)'}
"""


class Planner:
    def __init__(self, model: str, client: OpenAI | None = None):
        if client is None and not os.getenv("OPENAI_API_KEY"):
            raise PlannerError("OPENAI_API_KEY is not set.")
        self.model = model
        self.client = client or OpenAI()

    @retry(
        stop=stop_after_attempt(2), wait=wait_exponential(multiplier=1, min=1, max=4),
        retry=retry_if_exception_type((ValueError, TypeError)), reraise=True,
    )
    def plan(self, item: ProjectItem, context: RepositoryContext) -> HandoffPlan:
        try:
            response = self.client.responses.parse(
                model=self.model,
                instructions=SYSTEM_PROMPT,
                input=build_planning_input(item, context),
                text_format=HandoffPlan,
            )
            if response.output_parsed is None:
                raise ValueError("model returned no structured plan")
            return HandoffPlan.model_validate(response.output_parsed)
        except (ValueError, TypeError):
            raise
        except Exception as exc:
            raise PlannerError(f"OpenAI planning failed: {exc}") from exc

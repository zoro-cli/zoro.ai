"""Typer command-line interface."""

from __future__ import annotations

import os
import sys
from pathlib import Path

import questionary
import typer
from rich.console import Console
from rich.table import Table

from zoro import __version__
from zoro.codex import codex_available
from zoro.config import (
    HANDOFF_STATES,
    GitHubConfig,
    ZoroConfig,
    has_valid_repository,
    initialize,
    load_config,
)
from zoro.errors import ConfigError, ZoroError
from zoro.github import GitHubClient, resolve_token
from zoro.handoff import HandoffStore
from zoro.repository import (
    ensure_repository,
    executable_exists,
    git_remotes,
    git_status,
    repository_root,
    resolve_repository_identity,
)
from zoro.runner import Runner

app = typer.Typer(name="zoro", help="Local-first agentic development orchestrator.", no_args_is_help=True)
console = Console()


def _root() -> Path:
    return repository_root(Path.cwd())


def _runner() -> Runner:
    root = _root()
    return Runner(root, load_config(root))


def _display_error(exc: Exception) -> None:
    console.print(f"[bold red]Error:[/bold red] {exc}")


@app.callback(invoke_without_command=True)
def main(
    version: bool = typer.Option(
        False, "--version", help="Show version and exit.", is_eager=True
    ),
    verbose: bool = typer.Option(False, "--verbose", help="Enable verbose output."),
) -> None:
    if version:
        console.print(__version__)
        raise typer.Exit()


@app.command()
def init(project: int | None = typer.Option(None, "--project", min=1, help="GitHub Project number (avoids an interactive selection).")) -> None:
    """Initialize Zoro in the current repository."""
    try:
        cwd = Path.cwd()
        root = repository_root(cwd)
        remotes = git_remotes(root)
        remote_name = None
        if "origin" not in remotes and len(remotes) > 1:
            remote_name = questionary.select("Select the GitHub remote:", choices=list(remotes)).ask()
            if remote_name is None:
                raise typer.Abort()
        identity = resolve_repository_identity(cwd, remote_name)
        console.print("[bold]Repository[/bold]")
        console.print(f"  [green]✓[/green] Root: {root}")
        console.print(f"  [green]✓[/green] Remote: {identity.remote_url}")
        console.print(f"  [green]✓[/green] {identity.owner}/{identity.repo}")

        config_path = root / ".zoro/config.yaml"
        existing = None
        if config_path.exists():
            existing = load_config(root)
            incomplete = not has_valid_repository(existing.github)
            prompt = "Repair incomplete Zoro configuration?" if incomplete else "zoro.ai is already initialized. Reinitialize?"
            default = incomplete
            if not questionary.confirm(prompt, default=default).ask():
                console.print("Configuration unchanged.")
                return

        base = existing or ZoroConfig()
        provisional = base.model_copy(update={"github": GitHubConfig(
            owner=identity.owner, repo=identity.repo, project_number=project or base.github.project_number,
            status_field=base.github.status_field, statuses=base.github.statuses,
        )})
        client = GitHubClient(provisional.github)
        login = client.authenticated_user()
        client.verify_repository()
        console.print("[bold]GitHub[/bold]")
        console.print(f"  [green]✓[/green] Logged in as {login}")
        console.print("  [green]✓[/green] Repository access verified")

        projects = client.list_projects()
        if not projects:
            raise ConfigError(f"No accessible GitHub Projects were found for {identity.owner}.")
        if project is not None:
            selected = next((item for item in projects if item.number == project), None)
            if selected is None:
                raise ConfigError(f"GitHub Project #{project} is not accessible for {identity.owner}.")
        else:
            choice = questionary.select(
                "Select a GitHub Project:",
                choices=[questionary.Choice(f"{item.title} (#{item.number})", value=item) for item in projects],
            ).ask()
            if choice is None:
                raise typer.Abort()
            selected = choice
        final = provisional.model_copy(update={"github": provisional.github.model_copy(update={"project_number": selected.number})})
        validation_client = GitHubClient(final.github, token=client.token)
        project_info = validation_client.get_project()
        client.close()
        validation_client.close()
        created = initialize(root, final, overwrite=config_path.exists())
        console.print(f"[green]Initialized zoro.ai[/green] — {project_info.title} (#{selected.number})")
        for path in created:
            console.print(f"  {path.relative_to(root)}")
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def auth() -> None:
    """Verify GitHub repository and Project access."""
    try:
        root = _root()
        config = load_config(root)
        if not has_valid_repository(config.github):
            identity = resolve_repository_identity(root)
            config = config.model_copy(update={"github": config.github.model_copy(update={"owner": identity.owner, "repo": identity.repo})})
        client = GitHubClient(config.github)
        client.verify_repository()
        project = client.get_project()
        console.print(f"[green]Authenticated[/green] — {project.title}")
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def doctor() -> None:
    """Diagnose the local environment and configured integrations."""
    root = _root()
    checks: list[tuple[str, str, str]] = []

    def check(name: str, function, warning: bool = False) -> None:
        try:
            detail = function()
            checks.append((name, "!" if warning and detail else "✓", str(detail or "")))
        except Exception as exc:
            checks.append((name, "✗", str(exc)))

    check("Configuration", lambda: load_config(root) and "valid")
    check("Git repository", lambda: ensure_repository(root) or "detected")
    check("Git executable", lambda: "available" if executable_exists("git") else (_ for _ in ()).throw(RuntimeError("not found")))
    check("Repository clean", lambda: git_status(root) or "clean", warning=True)
    check("GitHub CLI", lambda: "available" if executable_exists("gh") else "not installed", warning=True)
    check("GitHub credentials", lambda: resolve_token() and "resolved")
    try:
        config = load_config(root)
        client = GitHubClient(config.github)
        check("GitHub repository", lambda: client.verify_repository() or "accessible")
        check("GitHub project", lambda: client.get_project().title)
    except Exception as exc:
        checks.append(("GitHub project", "✗", str(exc)))
    check("OpenAI API key", lambda: "set" if os.getenv("OPENAI_API_KEY") else (_ for _ in ()).throw(RuntimeError("OPENAI_API_KEY is not set")))
    check("Codex CLI", lambda: "available" if codex_available() else (_ for _ in ()).throw(RuntimeError("not found")))
    check("Python", lambda: f"{sys.version_info.major}.{sys.version_info.minor}" if sys.version_info >= (3, 12) else (_ for _ in ()).throw(RuntimeError("3.12+ required")))
    check("Handoff directories", lambda: "present" if all((root / "handoff" / state).is_dir() for state in HANDOFF_STATES) else (_ for _ in ()).throw(RuntimeError("run 'zoro init'")))
    table = Table(title="zoro.ai doctor")
    table.add_column("Check")
    table.add_column("")
    table.add_column("Detail")
    for row in checks:
        table.add_row(*row)
    console.print(table)
    if any(status == "✗" for _, status, _ in checks):
        raise typer.Exit(1)


@app.command()
def board() -> None:
    """Show project workflow counts."""
    try:
        runner = _runner()
        project = runner.github.get_project()
        counts = runner.github.board_counts(project)
        for label in runner.config.github.statuses.model_dump().values():
            console.print(f"{label:<16} {counts[label]}")
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def ready() -> None:
    """Show ordered Ready items."""
    try:
        runner = _runner()
        items = runner.github.ready_items()
        console.print("[bold]Ready[/bold]\n")
        for index, item in enumerate(items, 1):
            identity = f"#{item.issue_number}" if item.issue_number else "[draft]"
            console.print(f"{index}. {identity} {item.title}")
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def plan(issue: int | None = typer.Argument(None)) -> None:
    """Plan the top Ready item or a specific issue."""
    try:
        runner = _runner()
        _, item = runner.select_item(issue)
        existing = runner.store.find(item.issue_number, item.id)
        path = runner.plan_item(item)
        verb = "Already planned" if existing else "Handoff created"
        console.print(f"[green]{verb}[/green] {path.relative_to(_root())}")
    except (ZoroError, LookupError) as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def implement(issue: int | None = typer.Argument(None)) -> None:
    """Select and implement a ready handoff using Codex."""
    try:
        runner = _runner()
        files = runner.store.ready()
        if issue is not None:
            handoff = next((path for path in files if path.name.startswith(f"{issue}-")), None)
            if handoff is None:
                raise LookupError(f"No ready handoff exists for issue #{issue}.")
        else:
            if not files:
                raise LookupError("No handoffs are ready.")
            selected = questionary.select(
                "Select a handoff to implement:", choices=[path.name for path in files]
            ).ask()
            if selected is None:
                raise typer.Abort()
            handoff = next(path for path in files if path.name == selected)
        result = runner.implement(handoff)
        console.print(f"[green]Implementation ready for review[/green] {result.relative_to(_root())}")
    except (ZoroError, LookupError) as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command("run")
def run_command(once: bool = typer.Option(False, "--once", help="Run exactly one polling cycle.")) -> None:
    """Run one cycle or continuously poll the configured project."""
    try:
        runner = _runner()
        if once:
            result = runner.run_once()
            console.print(f"[green]Handoff created[/green] {result.relative_to(_root())}" if result else "No eligible Ready item.")
        else:
            console.print(f"Polling every {runner.config.scheduler.interval}. Press Ctrl+C to stop.")
            runner.run_forever(lambda path: console.print(f"Handoff created: {path.relative_to(_root())}") if path else None)
    except KeyboardInterrupt:
        console.print("Stopped.")
    except (ZoroError, LookupError) as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command()
def status() -> None:
    """Show local handoff lifecycle counts."""
    try:
        config = load_config(_root())
        store = HandoffStore(_root(), config.handoff.directory)
        for state in HANDOFF_STATES:
            console.print(f"{state.title():<16} {len(list((store.base / state).glob('*.md')))}")
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


@app.command("config")
def config_command() -> None:
    """Show the configuration path and validated non-secret settings."""
    try:
        config = load_config(_root())
        console.print(f"[bold]{(_root() / '.zoro/config.yaml')}[/bold]")
        console.print_json(data=config.model_dump())
    except ZoroError as exc:
        _display_error(exc)
        raise typer.Exit(1) from exc


if __name__ == "__main__":
    app()

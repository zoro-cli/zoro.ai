"""User-facing error hierarchy."""


class ZoroError(Exception):
    """Base error for expected operational failures."""


class ConfigError(ZoroError):
    pass


class AuthError(ZoroError):
    pass


class GitHubError(ZoroError):
    pass


class ProjectError(ZoroError):
    pass


class RepositoryError(ZoroError):
    pass


class PlannerError(ZoroError):
    pass


class HandoffError(ZoroError):
    pass


class CodexError(ZoroError):
    pass


class ValidationError(ZoroError):
    pass


class LockError(ZoroError):
    pass

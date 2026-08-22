package app

import "errors"

var (
	ErrConfig     = errors.New("config error")
	ErrAuth       = errors.New("authentication error")
	ErrGitHub     = errors.New("github error")
	ErrProject    = errors.New("project error")
	ErrRepository = errors.New("repository error")
	ErrPlanner    = errors.New("planner error")
	ErrHandoff    = errors.New("handoff error")
	ErrCodex      = errors.New("codex error")
	ErrValidation = errors.New("validation error")
	ErrLock       = errors.New("lock error")
)

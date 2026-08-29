package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

const (
	WorkbenchOutputRoot     = sandbox.SessionOutputRoot
	defaultWorkbenchTimeout = 5 * time.Minute
	maxWorkbenchTimeout     = 30 * time.Minute
	maxWorkbenchUploadBytes = 50 * 1024 * 1024
	maxRememberedCommands   = 256
)

type WorkbenchCommandStatus string

const (
	WorkbenchCommandRunning  WorkbenchCommandStatus = "running"
	WorkbenchCommandComplete WorkbenchCommandStatus = "complete"
	WorkbenchCommandCanceled WorkbenchCommandStatus = "canceled"
	WorkbenchCommandFailed   WorkbenchCommandStatus = "failed"
)

type WorkbenchCommandEvent struct {
	Type       string                 `json:"type"`
	CommandID  string                 `json:"command_id"`
	Stream     string                 `json:"stream,omitempty"`
	Data       string                 `json:"data,omitempty"`
	Status     WorkbenchCommandStatus `json:"status,omitempty"`
	ExitCode   int                    `json:"exit_code,omitempty"`
	Killed     bool                   `json:"killed,omitempty"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
}

type WorkbenchCommand struct {
	ID         string                 `json:"id"`
	SessionID  string                 `json:"session_id"`
	Status     WorkbenchCommandStatus `json:"status"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt *time.Time             `json:"finished_at,omitempty"`
	Result     *sandbox.ExecuteResult `json:"-"`
	stdout     string
	stderr     string
	cancel     context.CancelFunc
	done       chan struct{}
	subs       map[chan WorkbenchCommandEvent]struct{}
	mu         sync.Mutex
}

// SessionWorkbenchService is the application-level façade for the visual
// terminal and artifact file manager. It deliberately resolves the sandbox
// from the session pin, so switching an agent's configured backend cannot
// redirect an already-running session to another provider account.
type SessionWorkbenchService struct {
	sessions interfaces.SessionService
	resolver sandbox.TenantSandboxResolver
	fallback sandbox.Manager
	pinner   *SessionSandboxPinner
	audit    interfaces.AuditLogService

	mu       sync.RWMutex
	commands map[string]*WorkbenchCommand
	order    []string
}

func NewSessionWorkbenchService(
	sessions interfaces.SessionService,
	resolver sandbox.TenantSandboxResolver,
	fallback sandbox.Manager,
	pinner *SessionSandboxPinner,
	audit interfaces.AuditLogService,
) *SessionWorkbenchService {
	return &SessionWorkbenchService{sessions: sessions, resolver: resolver, fallback: fallback,
		pinner: pinner, audit: audit, commands: make(map[string]*WorkbenchCommand)}
}

func (s *SessionWorkbenchService) manager(ctx context.Context, sessionID string) (sandbox.Manager, error) {
	if _, err := s.sessions.GetOwnedSession(ctx, sessionID); err != nil {
		return nil, err
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, errors.New("workbench: tenant identity missing")
	}
	configID := ""
	if s.pinner != nil {
		var err error
		configID, err = s.pinner.Read(ctx, sessionID)
		if err != nil {
			return nil, err
		}
	}
	if s.resolver != nil {
		mgr, err := s.resolver.Resolve(ctx, tenantID, configID)
		if err != nil {
			return nil, err
		}
		if mgr != nil {
			return mgr, nil
		}
	}
	if s.fallback == nil {
		return nil, errors.New("workbench: sandbox unavailable")
	}
	return s.fallback, nil
}

func (s *SessionWorkbenchService) StartCommand(
	ctx context.Context, sessionID, command, workDir string, timeout time.Duration,
) (*WorkbenchCommand, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("command is required")
	}
	if len(command) > 16*1024 {
		return nil, errors.New("command is too long")
	}
	mgr, err := s.manager(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	provider, ok := mgr.(sandbox.SessionCapabilityProvider)
	if !ok || provider.SessionShellExecutor() == nil {
		return nil, errors.New("workbench: sandbox does not support terminal sessions")
	}
	workDir, err = cleanWorkbenchDir(workDir)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = defaultWorkbenchTimeout
	}
	if timeout > maxWorkbenchTimeout {
		timeout = maxWorkbenchTimeout
	}

	base := context.WithoutCancel(ctx)
	runCtx, cancel := context.WithTimeout(base, timeout)
	cmd := &WorkbenchCommand{ID: uuid.NewString(), SessionID: sessionID,
		Status: WorkbenchCommandRunning, StartedAt: time.Now().UTC(), cancel: cancel,
		done: make(chan struct{}), subs: make(map[chan WorkbenchCommandEvent]struct{})}
	s.remember(cmd)
	go s.runCommand(runCtx, provider.SessionShellExecutor(), provider.SessionFileStore(), cmd, command, workDir, timeout)
	return cmd, nil
}

func (s *SessionWorkbenchService) runCommand(ctx context.Context, executor sandbox.SessionShellExecutor,
	files sandbox.SessionFileStore, cmd *WorkbenchCommand, command, workDir string, timeout time.Duration) {
	defer cmd.cancel()
	// The provider-neutral Exec API returns a completed result rather than a
	// pipe. Redirecting into two session files and tailing them through the
	// equally neutral file API gives Docker, Cube and E2B identical live-output
	// semantics without leaking provider SDK stream types upward.
	tmpDir := path.Join(WorkbenchOutputRoot, ".weknora-terminal", cmd.ID)
	stdoutPath, stderrPath := path.Join(tmpDir, "stdout"), path.Join(tmpDir, "stderr")
	wrapper := fmt.Sprintf("mkdir -p %s && ( %s ) > %s 2> %s",
		sandbox.ShellQuote(tmpDir), command, sandbox.ShellQuote(stdoutPath), sandbox.ShellQuote(stderrPath))
	type execOutcome struct {
		result *sandbox.ExecuteResult
		err    error
	}
	finished := make(chan execOutcome, 1)
	go func() {
		result, err := executor.ExecShellCommand(ctx, cmd.SessionID, wrapper, workDir, timeout, nil)
		finished <- execOutcome{result: result, err: err}
	}()

	var stdout, stderr string
	poll := func(filePath, stream string, previous *string) {
		if files == nil {
			return
		}
		data, readErr := files.ReadSessionFile(ctx, cmd.SessionID, filePath)
		if readErr != nil {
			return
		}
		current := string(data)
		if len(current) < len(*previous) {
			*previous = ""
		}
		if len(current) > len(*previous) {
			cmd.publish(WorkbenchCommandEvent{Type: "output", CommandID: cmd.ID,
				Stream: stream, Data: current[len(*previous):]})
		}
		*previous = current
	}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	var outcome execOutcome
	for {
		select {
		case outcome = <-finished:
			poll(stdoutPath, "stdout", &stdout)
			poll(stderrPath, "stderr", &stderr)
			goto executionDone
		case <-ticker.C:
			poll(stdoutPath, "stdout", &stdout)
			poll(stderrPath, "stderr", &stderr)
		}
	}

executionDone:
	result, err := outcome.result, outcome.err
	if result == nil {
		result = &sandbox.ExecuteResult{ExitCode: -1}
	}
	result.Stdout, result.Stderr = stdout, stderr
	if outputFiles, ok := files.(sandbox.SessionOutputFileStore); ok {
		_ = outputFiles.RemoveSessionOutputPath(context.WithoutCancel(ctx), cmd.SessionID, tmpDir)
	}
	status := WorkbenchCommandComplete
	if errors.Is(ctx.Err(), context.Canceled) {
		status = WorkbenchCommandCanceled
	} else if err != nil || result.ExitCode != 0 || result.Killed {
		status = WorkbenchCommandFailed
	}
	now := time.Now().UTC()
	cmd.mu.Lock()
	cmd.Result, cmd.Status, cmd.FinishedAt = result, status, &now
	cmd.mu.Unlock()
	cmd.publish(WorkbenchCommandEvent{Type: "complete", CommandID: cmd.ID, Status: status,
		ExitCode: result.ExitCode, Killed: result.Killed, DurationMS: result.Duration.Milliseconds()})
	close(cmd.done)
	s.auditCommand(ctx, cmd, command, workDir, err)
}

func (s *SessionWorkbenchService) auditCommand(ctx context.Context, cmd *WorkbenchCommand,
	command, workDir string, runErr error) {
	if s.audit == nil {
		return
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)
	outcome := types.AuditOutcomeSuccess
	if cmd.Status == WorkbenchCommandCanceled {
		outcome = types.AuditOutcomeCanceled
	} else if cmd.Status == WorkbenchCommandFailed {
		outcome = types.AuditOutcomeFailed
	}
	detailMap := map[string]any{"command_id": cmd.ID, "command": command, "work_dir": workDir}
	if cmd.Result != nil {
		detailMap["exit_code"] = cmd.Result.ExitCode
		detailMap["duration_ms"] = cmd.Result.Duration.Milliseconds()
		detailMap["killed"] = cmd.Result.Killed
	}
	if runErr != nil {
		detailMap["error"] = runErr.Error()
	}
	details, _ := json.Marshal(detailMap)
	_ = s.audit.Log(context.WithoutCancel(ctx), &types.AuditLog{TenantID: tenantID,
		ActorUserID: userID, ActorRole: string(types.TenantRoleFromContext(ctx)),
		Action: types.AuditActionSandboxCommandExecuted, ScopeType: "session", ScopeID: cmd.SessionID,
		TargetType: "sandbox_command", TargetID: cmd.ID, Outcome: outcome, Details: types.JSON(details)})
}

func (s *SessionWorkbenchService) remember(cmd *WorkbenchCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands[cmd.ID] = cmd
	s.order = append(s.order, cmd.ID)
	for len(s.order) > maxRememberedCommands {
		old := s.order[0]
		item := s.commands[old]
		if item != nil && item.Status == WorkbenchCommandRunning {
			break
		}
		s.order = s.order[1:]
		delete(s.commands, old)
	}
}

func (s *SessionWorkbenchService) Command(ctx context.Context, sessionID, commandID string) (*WorkbenchCommand, error) {
	if _, err := s.sessions.GetOwnedSession(ctx, sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	cmd := s.commands[commandID]
	s.mu.RUnlock()
	if cmd == nil || cmd.SessionID != sessionID {
		return nil, errors.New("workbench command not found")
	}
	return cmd, nil
}

func (s *SessionWorkbenchService) CancelCommand(ctx context.Context, sessionID, commandID string) error {
	cmd, err := s.Command(ctx, sessionID, commandID)
	if err != nil {
		return err
	}
	cmd.mu.Lock()
	defer cmd.mu.Unlock()
	if cmd.Status == WorkbenchCommandRunning {
		cmd.cancel()
	}
	return nil
}

func (c *WorkbenchCommand) Subscribe() (<-chan WorkbenchCommandEvent, func()) {
	ch := make(chan WorkbenchCommandEvent, 8)
	c.mu.Lock()
	if c.Status != WorkbenchCommandRunning {
		result := c.Result
		e := WorkbenchCommandEvent{Type: "complete", CommandID: c.ID, Status: c.Status}
		if result != nil {
			e.ExitCode, e.Killed, e.DurationMS = result.ExitCode, result.Killed, result.Duration.Milliseconds()
			if result.Stdout != "" {
				ch <- WorkbenchCommandEvent{Type: "output", CommandID: c.ID, Stream: "stdout", Data: result.Stdout}
			}
			if result.Stderr != "" {
				ch <- WorkbenchCommandEvent{Type: "output", CommandID: c.ID, Stream: "stderr", Data: result.Stderr}
			}
		}
		ch <- e
		close(ch)
		c.mu.Unlock()
		return ch, func() {}
	}
	c.subs[ch] = struct{}{}
	if c.stdout != "" {
		ch <- WorkbenchCommandEvent{Type: "output", CommandID: c.ID, Stream: "stdout", Data: c.stdout}
	}
	if c.stderr != "" {
		ch <- WorkbenchCommandEvent{Type: "output", CommandID: c.ID, Stream: "stderr", Data: c.stderr}
	}
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		if _, ok := c.subs[ch]; ok {
			delete(c.subs, ch)
			close(ch)
		}
		c.mu.Unlock()
	}
}

func (c *WorkbenchCommand) publish(e WorkbenchCommandEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.Type == "output" {
		if e.Stream == "stderr" {
			c.stderr += e.Data
		} else {
			c.stdout += e.Data
		}
	}
	for ch := range c.subs {
		select {
		case ch <- e:
		default:
		}
		if e.Type == "complete" {
			close(ch)
			delete(c.subs, ch)
		}
	}
}

func (s *SessionWorkbenchService) FileStore(ctx context.Context, sessionID string) (sandbox.SessionFileStore, error) {
	mgr, err := s.manager(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	p, ok := mgr.(sandbox.SessionCapabilityProvider)
	if !ok || p.SessionFileStore() == nil {
		return nil, errors.New("workbench: sandbox does not support files")
	}
	return p.SessionFileStore(), nil
}

func CleanWorkbenchOutputPath(raw string, allowRoot bool) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || raw == "." {
		raw = WorkbenchOutputRoot
	}
	clean := path.Clean(raw)
	if !path.IsAbs(clean) {
		clean = path.Join(WorkbenchOutputRoot, clean)
	}
	if clean != WorkbenchOutputRoot && !strings.HasPrefix(clean, WorkbenchOutputRoot+"/") {
		return "", fmt.Errorf("path must stay under %s", WorkbenchOutputRoot)
	}
	if !allowRoot && clean == WorkbenchOutputRoot {
		return "", errors.New("output root cannot be modified")
	}
	return clean, nil
}

func cleanWorkbenchDir(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return sandbox.SessionWorkspaceRoot, nil
	}
	clean := path.Clean(strings.ReplaceAll(raw, "\\", "/"))
	if !path.IsAbs(clean) {
		clean = path.Join(sandbox.SessionWorkspaceRoot, clean)
	}
	if clean != sandbox.SessionWorkspaceRoot && !strings.HasPrefix(clean, sandbox.SessionWorkspaceRoot+"/") {
		return "", errors.New("work directory must stay under /workspace")
	}
	return clean, nil
}

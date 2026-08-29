package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

func TestCleanWorkbenchOutputPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		allowRoot bool
		want      string
		wantErr   bool
	}{
		{name: "root", input: "", allowRoot: true, want: "/workspace/output"},
		{name: "relative", input: "reports/q1.pptx", want: "/workspace/output/reports/q1.pptx"},
		{name: "absolute child", input: "/workspace/output/chart.html", want: "/workspace/output/chart.html"},
		{name: "parent traversal", input: "../input/secret.txt", wantErr: true},
		{name: "absolute outside", input: "/etc/passwd", wantErr: true},
		{name: "windows separators", input: "..\\input\\secret.txt", wantErr: true},
		{name: "root mutation", input: "/workspace/output", allowRoot: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanWorkbenchOutputPath(tt.input, tt.allowRoot)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCleanWorkbenchDir(t *testing.T) {
	t.Parallel()
	got, err := cleanWorkbenchDir("output/reports")
	require.NoError(t, err)
	require.Equal(t, "/workspace/output/reports", got)
	_, err = cleanWorkbenchDir("/opt/skills")
	require.Error(t, err)
}

func TestWorkbenchCommandLookupIsSessionScoped(t *testing.T) {
	t.Parallel()
	svc := &SessionWorkbenchService{sessions: ownedSessionStub{}, commands: map[string]*WorkbenchCommand{
		"cmd-1": {ID: "cmd-1", SessionID: "session-a"},
	}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	_, err := svc.Command(ctx, "session-b", "cmd-1")
	require.Error(t, err)
}

type ownedSessionStub struct{ interfaces.SessionService }

func (ownedSessionStub) GetOwnedSession(context.Context, string) (*types.Session, error) {
	return &types.Session{}, nil
}

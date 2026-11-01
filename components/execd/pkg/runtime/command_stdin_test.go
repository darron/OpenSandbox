// Copyright 2026 The OpenSandbox Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows
// +build !windows

package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/stretchr/testify/require"
)

// runWithStdin runs code in the foreground or background path and returns
// its combined output and exit code once the command has finished.
func runWithStdin(t *testing.T, background bool, code string, stdin []byte) (string, int) {
	t.Helper()
	c := NewController("", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var id string
	var out strings.Builder
	var execErr *execute.ErrorOutput
	req := &ExecuteCodeRequest{
		Code:    code,
		Cwd:     t.TempDir(),
		Timeout: 10 * time.Second,
		Stdin:   stdin,
		Hooks: ExecuteResultHook{
			OnExecuteInit:   func(s string) { id = s },
			OnExecuteStdout: func(s string) { out.WriteString(s) },
			OnExecuteStderr: func(s string) { out.WriteString(s) },
			OnExecuteError:  func(e *execute.ErrorOutput) { execErr = e },
		},
	}
	req.SetDefaultHooks()
	if background {
		require.NoError(t, c.runBackgroundCommand(ctx, cancel, req))
	} else {
		require.NoError(t, c.runCommand(ctx, req))
	}
	require.Eventually(t, func() bool {
		status, err := c.GetCommandStatus(id)
		return err == nil && !status.Running
	}, 10*time.Second, 10*time.Millisecond)
	status, err := c.GetCommandStatus(id)
	require.NoError(t, err)
	require.Nil(t, execErr)
	if background {
		data, _, err := c.SeekBackgroundCommandOutput(id, 0)
		require.NoError(t, err)
		return string(data), *status.ExitCode
	}
	return out.String(), *status.ExitCode
}

func TestCommandStdin(t *testing.T) {
	requireBash(t)
	// Larger than a pipe buffer, with NUL and non-UTF-8 bytes.
	big := bytes.Repeat([]byte("ab\x00\xff\n"), 40000)
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprintf("background=%v", background), func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)

			out, code := runWithStdin(t, background, `cat`, []byte("hello\nworld\n"))
			require.Equal(t, 0, code)
			require.Equal(t, "hello\nworld\n", out)

			out, code = runWithStdin(t, background, `wc -c | tr -d ' '`, big)
			require.Equal(t, 0, code)
			require.Equal(t, fmt.Sprintf("%d\n", len(big)), out)

			// Empty but present stdin is an immediate EOF.
			out, code = runWithStdin(t, background, `cat; echo done`, []byte{})
			require.Equal(t, 0, code)
			require.Equal(t, "done\n", out)

			// Absent stdin keeps the /dev/null behaviour.
			out, code = runWithStdin(t, background, `cat; echo done`, nil)
			require.Equal(t, 0, code)
			require.Equal(t, "done\n", out)

			// A command that ignores its stdin still finishes.
			out, code = runWithStdin(t, background, `echo ignored`, big)
			require.Equal(t, 0, code)
			require.Equal(t, "ignored\n", out)

			entries, err := os.ReadDir(tmp)
			require.NoError(t, err)
			for _, e := range entries {
				require.False(t, strings.HasPrefix(e.Name(), "execd-stdin-"), "stdin file left behind: %s", e.Name())
			}
		})
	}
}

func TestOpenCommandStdinReadOnly(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	f, err := openCommandStdin([]byte("x"))
	require.NoError(t, err)
	defer f.Close()
	_, err = f.Write([]byte("y"))
	require.Error(t, err, "the child must get a read-only stdin")
	nilFile, err := openCommandStdin(nil)
	require.NoError(t, err)
	require.Nil(t, nilFile)
}

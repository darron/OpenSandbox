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

package runtime

import (
	"fmt"
	"os"
)

// openCommandStdin returns a read-only file positioned at the start of data,
// or nil when the request supplied no stdin. The data lives in an unlinked
// temporary file, so the child gets a real *os.File (no copy goroutine for
// the reaper path to outlive), reads it and then EOF, and nothing is left on
// disk once every descriptor closes. The caller closes the returned file
// after the command starts or fails to start.
func openCommandStdin(data []byte) (*os.File, error) {
	if data == nil {
		return nil, nil
	}
	w, err := os.CreateTemp("", "execd-stdin-*")
	if err != nil {
		return nil, err
	}
	name := w.Name()
	defer os.Remove(name)
	_, err = w.Write(data)
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("write stdin: %w", err)
	}
	r, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(name); err != nil {
		_ = r.Close()
		return nil, fmt.Errorf("unlink stdin: %w", err)
	}
	return r, nil
}

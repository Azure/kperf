//go:build !linux

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package mountns

import "fmt"

func Executes(run func() error) error {
	return fmt.Errorf("not supported")
}

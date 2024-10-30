// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package localstore

import "os"

// sizeReadCloser implements Reader interface.
type sizeReadCloser struct {
	*os.File
	size int64
}

// Size returns file's size.
func (r *sizeReadCloser) Size() int64 {
	return r.size
}

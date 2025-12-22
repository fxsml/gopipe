package message

import "errors"

// ErrAlreadyStarted is returned when Start is called on an already-started router.
var ErrAlreadyStarted = errors.New("message: already started")

package pipe

import "errors"

// ErrAlreadyStarted is returned when Start or ApplyMiddleware is called
// on a pipe that has already been started.
var ErrAlreadyStarted = errors.New("pipe: already started")

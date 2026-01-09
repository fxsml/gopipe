package pipe

import "errors"

// ErrAlreadyStarted is returned when Start or Use is called
// on a pipe that has already been started.
var ErrAlreadyStarted = errors.New("pipe: already started")

// ErrNoMatchingOutput is returned when no output matches a message in the distributor.
var ErrNoMatchingOutput = errors.New("distributor: no matching output")

// ErrShutdownDropped is returned when a message is dropped due to shutdown.
var ErrShutdownDropped = errors.New("shutdown: message dropped")

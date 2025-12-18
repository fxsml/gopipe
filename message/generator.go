package message

import "github.com/fxsml/gopipe/pipe"

type Generator interface {
	pipe.Generator[*Message]
}
